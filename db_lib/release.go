package db_lib

import (
	"errors"
	"os"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// prepareRelease verifies Git objects before checkout and submodule initialization.
// Callers must hold the checkout lock until the task finishes using its files.
func (r GitRepository) prepareRelease() error {
	if err := r.Repository.Validate(); err != nil {
		return err
	}
	if err := os.RemoveAll(r.GetFullPath()); err != nil {
		return err
	}
	if r.TmpDirName == "" {
		r.TmpDirName = r.Repository.GetCheckoutDirName(r.TemplateID)
	}
	if err := r.cloneReleaseObjects(); err != nil {
		return err
	}
	commit, err := r.verifyRelease()
	if err != nil {
		return err
	}
	return r.materializeRelease(commit)
}

func (r GitRepository) cloneReleaseObjects() error {
	ref := plumbing.ReferenceName(r.Repository.GetGitRef())
	switch c := r.Client.(type) {
	case CmdGitClient:
		// Full ref names avoid ambiguity when a branch and a tag share a name.
		if err := c.run(r, GitRepositoryTmpPath, "clone", "--no-checkout", "--no-recurse-submodules", "--", r.Repository.GetGitURL(true), r.TmpDirName); err != nil {
			return err
		}
		if err := c.run(r, GitRepositoryFullPath, "fetch", "--no-recurse-submodules", "origin", string(ref)); err != nil {
			return err
		}
		return c.run(r, GitRepositoryFullPath, "update-ref", "HEAD", "FETCH_HEAD^{commit}")
	case GoGitClient:
		auth, err := c.getAuthMethod(r)
		if err != nil {
			return err
		}
		_, err = git.PlainClone(r.GetFullPath(), false, &git.CloneOptions{
			URL: r.Repository.GetGitURL(false), Auth: auth, ReferenceName: ref,
			NoCheckout: true, Tags: git.AllTags,
		})
		return err
	default:
		return errors.New("release preparation requires a supported Git client")
	}
}

func (r GitRepository) verifyRelease() (plumbing.Hash, error) {
	repo, err := git.PlainOpen(r.GetFullPath())
	if err != nil {
		return plumbing.ZeroHash, err
	}
	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if !r.Repository.VerifySignature {
		return head.Hash(), nil
	}
	// Index tags by their peeled commit before checking signatures. Selection
	// must not skip a newer unsigned or untrusted release in favor of an old one.
	tags := make(map[plumbing.Hash][]*plumbing.Reference)
	refs, err := repo.Tags()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	defer refs.Close()
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if r.Repository.GitTag != "" && ref.Name() != plumbing.NewTagReferenceName(r.Repository.GitTag) {
			return nil
		}
		hash := ref.Hash()
		for depth := 0; depth < 32; depth++ {
			obj, err := repo.Object(plumbing.AnyObject, hash)
			if err != nil {
				return err
			}
			switch o := obj.(type) {
			case *object.Tag:
				hash = o.Target
			case *object.Commit:
				tags[hash] = append(tags[hash], ref)
				return nil
			default:
				return nil // Tags on trees and blobs are not releases.
			}
		}
		return errors.New("release tag nesting exceeds limit")
	})
	if err != nil {
		return plumbing.ZeroHash, err
	}
	selected := head.Hash()
	if r.Repository.GitTag == "" {
		commit, err := repo.CommitObject(selected)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		for len(tags[commit.Hash]) == 0 {
			if commit.NumParents() == 0 {
				return plumbing.ZeroHash, errors.New("no release tag on branch first-parent history")
			}
			commit, err = commit.Parent(0)
			if err != nil {
				return plumbing.ZeroHash, err
			}
		}
		selected = commit.Hash
	}
	for _, ref := range tags[selected] {
		tag, err := repo.TagObject(ref.Hash())
		if err != nil || tag.TargetType != plumbing.CommitObject || tag.Target != selected {
			continue
		}
		if _, err = tag.Verify(r.Repository.SigningKey); err != nil {
			continue
		}
		r.Logger.Log("Verified release tag " + tag.Name + " at " + selected.String())
		return selected, nil
	}
	return plumbing.ZeroHash, errors.New("latest release tag has no valid annotated signature from the trusted maintainer key")
}

func (r GitRepository) materializeRelease(hash plumbing.Hash) error {
	repo, err := git.PlainOpen(r.GetFullPath())
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err = wt.Checkout(&git.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return err
	}
	switch c := r.Client.(type) {
	case CmdGitClient:
		return c.run(r, GitRepositoryFullPath, "submodule", "update", "--init", "--recursive")
	case GoGitClient:
		auth, err := c.getAuthMethod(r)
		if err != nil {
			return err
		}
		modules, err := wt.Submodules()
		if err != nil {
			return err
		}
		return modules.Update(&git.SubmoduleUpdateOptions{Init: true, Auth: auth, RecurseSubmodules: git.DefaultSubmoduleRecursionDepth})
	}
	return errors.New("unsupported Git client")
}
