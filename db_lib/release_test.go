package db_lib

import (
	"bytes"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func releaseKey(t *testing.T) (*openpgp.Entity, string) {
	t.Helper()
	key, err := openpgp.NewEntity("Maintainer", "", "maintainer@example.com", nil)
	require.NoError(t, err)
	var buf bytes.Buffer
	writer, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	require.NoError(t, err)
	require.NoError(t, key.Serialize(writer))
	require.NoError(t, writer.Close())
	return key, buf.String()
}

func releaseTag(t *testing.T, dir, name, hash string, key *openpgp.Entity) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	_, err = repo.CreateTag(name, plumbing.NewHash(hash), &git.CreateTagOptions{
		Tagger:  &object.Signature{Name: "Maintainer", Email: "maintainer@example.com", When: time.Now()},
		Message: "Approved release", SignKey: key,
	})
	require.NoError(t, err)
}

func TestReleasePreparation(t *testing.T) {
	for _, clientName := range []string{"cmd", "go"} {
		t.Run(clientName, func(t *testing.T) {
			setupGitClientTest(t)
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			upstream := t.TempDir()
			gitInit(t, upstream)
			backend := filepath.Join(gitOut(t, upstream, "--exec-path"), "git-http-backend")
			require.FileExists(t, backend)
			server := httptest.NewServer(&cgi.Handler{Path: backend, Env: []string{
				"GIT_PROJECT_ROOT=" + filepath.Dir(upstream), "GIT_HTTP_EXPORT_ALL=1",
			}, InheritEnv: []string{"PATH", "USER", "SYSTEMROOT"}})
			defer server.Close()

			module := t.TempDir()
			gitInit(t, module)
			gitOut(t, upstream, "submodule", "add", server.URL+"/"+filepath.Base(module), "module")
			gitOut(t, upstream, "commit", "-qam", "add pinned submodule")
			first := gitOut(t, upstream, "rev-parse", "HEAD")
			trusted, public := releaseKey(t)
			releaseTag(t, upstream, "v1", first, trusted)
			r := newTestGitRepo(t, server.URL+"/"+filepath.Base(upstream), "main")
			r.Repository.Name = "Release"
			r.Repository.VerifySignature = true
			r.Repository.SigningKey = public
			r.TmpDirName = "approved"
			r.Client = CreateCmdGitClient(nopKeyInstaller{})
			if clientName == "go" {
				r.Client = CreateGoGitClient(nopKeyInstaller{})
			}

			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, first, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))
			content, readErr := os.ReadFile(filepath.Join(r.GetFullPath(), "module", "f"))
			require.NoError(t, readErr)
			assert.Equal(t, "hi", string(content))
			second := gitCommitFile(t, upstream, "unapproved", "must never be processed")
			require.NoError(t, r.CloneOrPull(), "unsigned branch commits after a release must not be checked out")
			assert.NoFileExists(t, filepath.Join(r.GetFullPath(), "unapproved"))

			// Lightweight tags and an attacker's signed annotated tags cannot approve HEAD.
			gitOut(t, upstream, "tag", "lightweight")
			releaseTag(t, upstream, "unsigned", second, nil)
			require.Error(t, r.CloneOrPull())
			attacker, _ := releaseKey(t)
			releaseTag(t, upstream, "forged", second, attacker)
			require.Error(t, r.CloneOrPull())
			// Copy a real signature onto a tag with a changed target.
			remote, e := git.PlainOpen(upstream)
			require.NoError(t, e)
			originalRef, e := remote.Tag("v1")
			require.NoError(t, e)
			altered, e := remote.TagObject(originalRef.Hash())
			require.NoError(t, e)
			altered.Target = plumbing.NewHash(second)
			encoded := &plumbing.MemoryObject{}
			require.NoError(t, altered.Encode(encoded))
			tampered, e := remote.Storer.SetEncodedObject(encoded)
			require.NoError(t, e)
			require.NoError(t, remote.Storer.SetReference(plumbing.NewHashReference("refs/tags/tampered", tampered)))
			require.Error(t, r.CloneOrPull(), "a previously verified checkout must never bypass a rejected release")

			// An unrelated maintainer key cannot approve the new release.
			_, unrelated := releaseKey(t)
			r.Repository.SigningKey = unrelated
			require.Error(t, r.CloneOrPull())
			r.Repository.SigningKey = public

			releaseTag(t, upstream, "v2", second, trusted)
			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, second, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))

			// Tag and branch names are resolved in separate namespaces.
			gitOut(t, upstream, "branch", "v1", second)
			r.Repository.GitBranch = ""
			r.Repository.GitTag = "v1"
			remoteHash, remoteErr := r.GetLastRemoteCommitHash()
			require.NoError(t, remoteErr)
			assert.Equal(t, first, remoteHash, "tag lookup must ignore a branch with the same name")
			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, first, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))

			gitOut(t, upstream, "tag", "-d", "v1")
			require.Error(t, r.CloneOrPull(), "a deleted tag must fail even after a successful run")

			// A fixed lightweight tag also works without verification.
			r.Repository.VerifySignature = false
			r.Repository.GitTag = "lightweight"
			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, second, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))
			r.Repository.VerifySignature = true
			r.Repository.GitTag = "v2"
			require.NoError(t, r.CloneOrPull())
			server.Close()
			require.Error(t, r.CloneOrPull(), "network outages must fail even after a successful run")

		})
	}
}

func TestReleaseRejectsBeforeSubmodules(t *testing.T) {
	setupGitClientTest(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	upstream := t.TempDir()
	gitInit(t, upstream)
	gitOut(t, upstream, "config", "http.uploadpack", "true")
	_, public := releaseKey(t)
	backend := filepath.Join(gitOut(t, upstream, "--exec-path"), "git-http-backend")
	server := httptest.NewServer(&cgi.Handler{Path: backend, Env: []string{
		"GIT_PROJECT_ROOT=" + filepath.Dir(upstream), "GIT_HTTP_EXPORT_ALL=1",
	}, InheritEnv: []string{"PATH", "USER", "SYSTEMROOT"}})
	defer server.Close()
	gitCommitFile(t, upstream, ".gitmodules", "[submodule \"evil\"]\npath = evil\nurl = /does/not/exist\n")
	gitOut(t, upstream, "update-index", "--add", "--cacheinfo", "160000,"+gitOut(t, upstream, "rev-parse", "HEAD")+",evil")
	gitOut(t, upstream, "commit", "-qm", "unsigned submodule")
	r := newTestGitRepo(t, server.URL+"/"+filepath.Base(upstream), "main")
	r.Repository.Name = "release"
	r.Repository.VerifySignature = true
	r.Repository.SigningKey = public
	r.Client = CreateCmdGitClient(nopKeyInstaller{})
	require.ErrorContains(t, r.CloneOrPull(), "release tag")
	assert.NoFileExists(t, filepath.Join(r.GetFullPath(), ".gitmodules"))
	stagedDir := filepath.Join(filepath.Dir(r.GetFullPath()), "objects")
	require.NoError(t, os.MkdirAll(stagedDir, 0700))
	r.TmpDirName = "objects"
	require.NoError(t, r.cloneReleaseObjects(), "unsigned submodule URLs are never contacted")
	assert.NoFileExists(t, filepath.Join(r.GetFullPath(), ".gitmodules"))
	_, err := r.verifyRelease()
	assert.ErrorContains(t, err, "release tag")
}

func TestReleasePreparation_FileTransport(t *testing.T) {
	for _, clientName := range []string{"cmd", "go"} {
		t.Run(clientName, func(t *testing.T) {
			setupGitClientTest(t)
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			upstream := t.TempDir()
			gitInit(t, upstream)
			first := gitOut(t, upstream, "rev-parse", "HEAD")
			trusted, public := releaseKey(t)
			releaseTag(t, upstream, "v1", first, trusted)
			r := newTestGitRepo(t, "file://"+filepath.ToSlash(upstream), "main")
			r.Repository.Name = "Local Git release"
			r.Repository.VerifySignature = true
			r.Repository.SigningKey = public
			r.Client = CreateCmdGitClient(nopKeyInstaller{})
			if clientName == "go" {
				r.Client = CreateGoGitClient(nopKeyInstaller{})
			}

			require.NoError(t, r.Repository.Validate())
			require.NoError(t, r.CloneOrPull())
			assert.NotEqual(t, upstream, r.GetFullPath())
			assert.Equal(t, first, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))

			second := gitCommitFile(t, upstream, "unsigned", "not approved")
			require.NoError(t, r.CloneOrPull(), "unsigned branch commits after a release must not be checked out")
			assert.NoFileExists(t, filepath.Join(r.GetFullPath(), "unsigned"))
			assert.Equal(t, second, gitOut(t, upstream, "rev-parse", "HEAD"), "source is never reset or checked out")
			require.NoError(t, r.CloneOrPull())

			// A tag on a merged side branch must not replace the release on
			// the configured branch's first-parent history.
			gitOut(t, upstream, "checkout", "-qb", "side", first)
			side := gitCommitFile(t, upstream, "side", "side branch")
			releaseTag(t, upstream, "side-release", side, trusted)
			gitOut(t, upstream, "checkout", "-q", "main")
			gitOut(t, upstream, "merge", "--no-ff", "side", "-m", "merge side branch")
			second = gitOut(t, upstream, "rev-parse", "HEAD")
			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, first, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))
			assert.NoFileExists(t, filepath.Join(r.GetFullPath(), "side"))

			gitOut(t, upstream, "tag", "unsigned-release")
			require.Error(t, r.CloneOrPull(), "a newer unsigned tag must fail instead of selecting the older signed release")

			r.Repository.GitBranch = ""
			r.Repository.GitTag = "v1"
			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, first, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))

			gitOut(t, upstream, "tag", "lightweight")
			r.Repository.VerifySignature = false
			r.Repository.GitTag = "lightweight"
			require.NoError(t, r.CloneOrPull())
			assert.Equal(t, second, gitOut(t, r.GetFullPath(), "rev-parse", "HEAD"))

			r.Repository.VerifySignature = true
			r.Repository.GitTag = "v1"
			require.NoError(t, r.CloneOrPull())
			require.NoError(t, os.Rename(upstream, filepath.Join(t.TempDir(), "offline")))
			require.Error(t, r.CloneOrPull(), "an unavailable source must fail even after a successful run")
			assert.NoFileExists(t, filepath.Join(r.GetFullPath(), "f"))
		})
	}
}
