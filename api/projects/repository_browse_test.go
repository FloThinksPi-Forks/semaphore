package projects

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/semaphoreui/semaphore/api/helpers"
	"github.com/semaphoreui/semaphore/db"
	"github.com/semaphoreui/semaphore/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryController_lockBrowseDir_SerializesSameDir(t *testing.T) {
	c := &RepositoryController{}

	unlock := c.lockBrowseDir("repository_1_browse_aaaa")

	acquired := make(chan struct{})
	go func() {
		second := c.lockBrowseDir("repository_1_browse_aaaa")
		close(acquired)
		second()
	}()

	select {
	case <-acquired:
		t.Fatal("second request entered the scratch checkout while the first still held it")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second request was not let in after the first released the checkout")
	}
}

func TestRepositoryController_lockBrowseDir_IndependentDirs(t *testing.T) {
	c := &RepositoryController{}

	unlockA := c.lockBrowseDir("repository_1_browse_aaaa")
	defer unlockA()

	acquired := make(chan struct{})
	go func() {
		unlockB := c.lockBrowseDir("repository_1_browse_bbbb")
		close(acquired)
		unlockB()
	}()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("a different scratch checkout was blocked by an unrelated lock")
	}
}

func TestRepositoryController_lockBrowseDir_Reentrant(t *testing.T) {
	c := &RepositoryController{}

	var wg sync.WaitGroup
	counter := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := c.lockBrowseDir("same")
			defer unlock()
			counter++
		}()
	}
	wg.Wait()

	assert.Equal(t, 20, counter)
}

type rejectedRepositoryKey struct{}

func (rejectedRepositoryKey) DeserializeSecret(*db.AccessKey) error {
	return errors.New("repository key rejected")
}

func TestGetRepositoryPlaybooks_FileURLRequiresPreparation(t *testing.T) {
	previous := util.Config
	util.Config = &util.ConfigType{TmpPath: t.TempDir()}
	t.Cleanup(func() { util.Config = previous })
	repo := db.Repository{ID: 1, ProjectID: 1, GitURL: "file:///source.git", GitBranch: "main", VerifySignature: true}
	// A cached working tree must not bypass preparation, even for file:// URLs.
	require.NoError(t, os.MkdirAll(repo.GetFullPath(0), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(repo.GetFullPath(0), "unverified.yml"), []byte("[]"), 0600))
	request := helpers.SetContextValue(httptest.NewRequest(http.MethodGet, "/playbooks?app=ansible", nil), "repository", repo)
	response := httptest.NewRecorder()
	controller := NewRepositoryController(nil, rejectedRepositoryKey{})
	controller.GetRepositoryPlaybooks(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.NotContains(t, response.Body.String(), "unverified.yml")
}
