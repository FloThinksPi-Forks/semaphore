package sql

import (
	"testing"

	"github.com/semaphoreui/semaphore/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration_2_20_19_ForkSignedReleases(t *testing.T) {
	before := "2.20.18"
	store := InitConfigCreateTestStoreAt(&before)
	projectID, repositoryID := newTemplateTestProject(t, store)
	// An upstream migration with the same numeric version is a different ID.
	_, err := store.exec("insert into migrations(version, upgraded_date) values (?, CURRENT_TIMESTAMP)", "2.20.19")
	require.NoError(t, err)
	target := "2.20.19.fork.signed-releases"
	require.NoError(t, db.Migrate(store, &target))
	repo, err := store.GetRepository(projectID, repositoryID)
	require.NoError(t, err)
	assert.False(t, repo.VerifySignature)
	assert.Empty(t, repo.GitTag)
	repo.GitBranch = ""
	repo.GitTag = "v1.2.3"
	// Public keys are retained even while verification is disabled.
	repo.SigningKey = "public key"
	require.NoError(t, store.UpdateRepository(repo))
	loaded, err := store.GetRepository(projectID, repositoryID)
	require.NoError(t, err)
	assert.Equal(t, repo, loaded)
	repo.ID = 0
	created, err := store.CreateRepository(repo)
	require.NoError(t, err)
	loaded, err = store.GetRepository(projectID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.SigningKey, loaded.SigningKey)
	assert.Equal(t, "v1.2.3", loaded.GitTag)
	repo.ID = repositoryID
	repo.SigningKey = ""
	require.NoError(t, store.UpdateRepository(repo))
	require.NoError(t, db.Rollback(store, "2.20.18"))
	require.NoError(t, db.Migrate(store, &target))
	loaded, err = store.GetRepository(projectID, repositoryID)
	require.NoError(t, err)
	assert.Empty(t, loaded.GitTag)
}
