// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo_harness

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/chromiumos/infra/go/internal/test_util"
	"go.chromium.org/luci/common/errors"
)

var (
	CommandRunnerImpl cmd.CommandRunner = cmd.RealCommandRunner{}
)

// A ProjectFile struct describes a file to be placed in a particular project.
type File struct {
	Project  string
	Name     string
	Contents []byte
	Perm     os.FileMode
}

const (
	readWritePerms = 0666
	dirPerms       = 0777
)

type RepoHarnessConfig struct {
	// Initialize() will create a test harness with
	// the appropriate remote repos and a local repo.
	// Both remote and local repos will have the appropriate
	// projects created (with initialized git repos inside them).
	Manifest repo.Manifest
}

type RepoHarness struct {
	// Manifest that defines the harness configuration.
	manifest repo.Manifest
	// Root directory of the whole harness setup.
	harnessRoot string
	// Local checkout.
	LocalRepo string
}

func (r *RepoHarness) runCommand(cmd []string, cwd string) error {
	if cwd == "" {
		cwd = r.harnessRoot
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var stdoutBuf, stderrBuf bytes.Buffer
	err := CommandRunnerImpl.RunCommand(ctx, &stdoutBuf, &stderrBuf, cwd, cmd[0], cmd[1:]...)
	if err != nil {
		return errors.Annotate(err, "error running command (stderr: %s)", stderrBuf.String()).Err()
	}
	return nil
}

func (r *RepoHarness) Initialize(config *RepoHarnessConfig) error {
	var err error
	// Set up root directory for harness instance.
	r.harnessRoot, err = ioutil.TempDir("", "repo_harness")
	if err != nil {
		return errors.Annotate(err, "failed to create harness root dir").Err()
	}
	// Create snapshots/ dir.
	if err = os.Mkdir(filepath.Join(r.harnessRoot, "snapshots"), dirPerms); err != nil {
		return errors.Annotate(err, "failed to create harness snapshots dir").Err()
	}

	// Resolve implicit links in the manifest. We do this so that each project has
	// an explicit remote listed.
	r.manifest = *config.Manifest.ResolveImplicitLinks()

	// Create local repository dir.
	r.LocalRepo = filepath.Join(r.harnessRoot, "local")
	err = os.Mkdir(r.LocalRepo, dirPerms)
	if err != nil {
		return errors.Annotate(err, "failed to create local repo dir").Err()
	}

	// Initialize remote repositories.
	for _, remote := range r.manifest.Remotes {
		remoteName := remote.Name
		// Create directory.
		err = os.Mkdir(filepath.Join(r.harnessRoot, remoteName), dirPerms)
		if err != nil {
			return errors.Annotate(err, "failed to create tmp dir for remote %s", remoteName).Err()
		}
	}
	// Update Fetch attribute in manifest remotes.
	for i, remote := range r.manifest.Remotes {
		r.manifest.Remotes[i].Fetch = "file://" + filepath.Join(r.harnessRoot, remote.Name)
	}

	// Initialize projects on remotes.
	for _, project := range r.manifest.Projects {
		remoteName := project.RemoteName
		projectPath := filepath.Join(r.harnessRoot, remoteName, project.Name)
		projectLabel := fmt.Sprintf("project %s (remote %s)", project.Name, remoteName)

		// Project could already exist due to multiple checkouts. If it does, skip
		// initialization/master branch setup.
		if _, err = os.Stat(projectPath); err != nil {
			// Create project directory.
			if err = os.MkdirAll(projectPath, dirPerms); err != nil {
				return errors.Annotate(err, "failed to create dir for %s", projectLabel).Err()
			}
			// Initialize bare repo in project directory.
			if err = git.Init(projectPath, true); err != nil {
				return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
			}

			// Make an initial commit so that the "master" branch is not unborn.
			if err = r.CreateRemoteRef(project, "master", ""); err != nil {
				return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
			}
		}
		// If revision is set, create that branch too.
		if project.Revision != "" && !strings.HasPrefix(project.Revision, "refs/heads/") {
			return fmt.Errorf("revisions must be of the form refs/heads/<branch>")
		}

		revision := git.StripRefs(project.Revision)
		if revision != "" && revision != "master" {
			if err = r.CreateRemoteRef(project, revision, ""); err != nil {
				return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
			}
		}
	}

	// Create local checkout of the project at r.LocalRepo, in case the client would like one.
	// Specifically, we create temporary git repo with the manifest and sync using
	// `repo sync`.
	// TOOD(@jackneus): Will syncing from (and then deleting) a temp repo make subsequent
	// `repo sync`s fail?
	// Answer: yes
	manifestRepo, err := ioutil.TempDir(r.harnessRoot, "manifest-repo")
	defer os.RemoveAll(manifestRepo)
	if err != nil {
		return errors.Annotate(err, "error initializing temp git repo for manifest").Err()
	}
	manifestPath := filepath.Join(manifestRepo, "default.xml")
	errs := []error{
		git.Init(manifestRepo, false),
		r.manifest.Write(manifestPath),
	}
	_, err = git.CommitAll(manifestRepo, "commit manifest")
	errs = append(errs,
		err,
		r.runCommand([]string{"repo", "init", "--manifest-url", manifestRepo}, r.LocalRepo),
	)

	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to initialize local checkout").Err()
		}
	}
	err = r.runCommand([]string{"repo", "sync"}, r.LocalRepo)
	if err != nil {
		return errors.Annotate(err, "failed to sync local checkout").Err()
	}

	return nil
}

func (r *RepoHarness) assertInitialized() error {
	if r.harnessRoot == "" {
		return fmt.Errorf("repo harness needs to be initialized")
	}
	return nil
}

func (r *RepoHarness) Teardown() error {
	if r.harnessRoot != "" {
		root := r.harnessRoot
		r.harnessRoot = ""
		return os.RemoveAll(root)
	}
	return fmt.Errorf("harness was never initialized")
}

// CreateRemoteRef creates a remote ref for a specific project.
// Otherwise, a temporary local checkout will be created and an empty commit
// will be used to create the remote ref.
func (r *RepoHarness) CreateRemoteRef(project repo.Project, ref, commit string) error {
	projectLabel := fmt.Sprintf("%s/%s", project.RemoteName, project.Name)
	remoteProjectPath := r.getRemotePath(project)

	var repoPath string
	var err error
	remoteRef := git.RemoteRef{
		Ref: git.NormalizeRef(ref),
	}

	if commit == "" {
		// Set up tmp local repo and make empty commit.
		repoPath, err = ioutil.TempDir(r.harnessRoot, "tmp-repo")
		defer os.RemoveAll(repoPath)
		errs := []error{
			err,
			git.Init(repoPath, false),
		}
		for _, err := range errs {
			if err != nil {
				return errors.Annotate(err, "failed to make temp local repo").Err()
			}
		}
		commitMsg := fmt.Sprintf("empty commit for ref %s %s", remoteRef.Remote, remoteRef.Ref)
		commit, err = git.CommitEmpty(repoPath, commitMsg)
		if err != nil {
			return errors.Annotate(err, "failed to make empty commit").Err()
		}

		if err = git.AddRemote(repoPath, project.RemoteName, remoteProjectPath); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				err = nil
			} else {
				return errors.Annotate(err, "failed to add remote %s to tmp repo", project.RemoteName).Err()
			}
		}
		remoteRef.Remote = project.RemoteName
	} else {
		repoPath = remoteProjectPath
		remoteRef.Remote = remoteProjectPath
	}

	if err := git.PushRef(repoPath, commit, false, remoteRef); err != nil {
		return errors.Annotate(err, "failed to add remote ref %s %s:%s", projectLabel, commit, remoteRef.Ref).Err()
	}
	return nil
}

// This is needed for projects with multiple checkouts.
// Returns the sha1 of the commit that adds the file.
func (r *RepoHarness) AddFile(file File) (string, error) {
	if err := r.assertInitialized(); err != nil {
		return "", err
	}

	project, err := r.manifest.GetProjectByPath(file.Project)
	if err != nil {
		return "", err
	}
	projectLabel := fmt.Sprintf("%s", project.Path)

	// Populate project in specified remote with files. Because the remote repository is bare,
	// we need to write/commit the files locally and then push them to the remote.
	// We do this using a temp checkout of the appropriate remote.
	tmpRepo := filepath.Join(r.harnessRoot, "tmp-repo")
	filePath := filepath.Join(tmpRepo, file.Name)
	if file.Perm == 0 {
		file.Perm = 0644
	}

	remoteRef := git.RemoteRef{
		Remote: "origin",
		Ref:    "master",
	}

	errs := []error{
		os.Mkdir(tmpRepo, dirPerms),
		git.Clone(r.getRemotePath(*project), tmpRepo),
		ioutil.WriteFile(filePath, file.Contents, file.Perm),
	}
	commit, err := git.PushChanges(tmpRepo, "master", "initial commit", false, remoteRef)
	errs = append(errs,
		err,
		os.RemoveAll(tmpRepo),
	)
	for _, err = range errs {
		if err != nil {
			return "", errors.Annotate(err, "failed to add file %s to %s", file.Name, projectLabel).Err()
		}
	}

	return commit, nil
}

// TODO(@jackneus): rewrite so that only one commit is made to each project.
// Return a map with the sha1's of the commits.
func (r *RepoHarness) AddFiles(files []File) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	for _, file := range files {
		_, err := r.AddFile(file)
		if err != nil {
			return err
		}
	}

	return nil
}

// Snapshot recursively copies a directory's contents to a temp dir.
func (r *RepoHarness) Snapshot(path string) (string, error) {
	snapshotRoot := filepath.Join(r.harnessRoot, "snapshots/")
	snapshotDir, err := ioutil.TempDir(snapshotRoot, "snapshot")
	if err != nil {
		return "", err
	}
	if err = copy.Copy(path, snapshotDir); err != nil {
		return "", err
	}
	return snapshotDir, nil
}

// getRemotePath gets the path to the remote project repo.
func (r *RepoHarness) getRemotePath(project repo.Project) string {
	return filepath.Join(r.harnessRoot, project.RemoteName, project.Name)
}

// AssertProjectBranches asserts that the remote project has the correct branches.
func (r *RepoHarness) AssertProjectBranches(project repo.Project, branches []string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	gitRepo := r.getRemotePath(project)
	return test_util.AssertGitBranches(gitRepo, branches)
}

// AssertProjectBranchesExact asserts that the remote project has only the correct branches.
func (r *RepoHarness) AssertProjectBranchesExact(project repo.Project, branches []string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	gitRepo := r.getRemotePath(project)
	return test_util.AssertGitBranchesExact(gitRepo, branches)
}

// AssertProjectBranchEqual asserts that the specified branch in the project matches
// the corresponding branch in the given snapshot.
func (r *RepoHarness) AssertProjectBranchEqual(project repo.Project, branch, snapshotPath string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	expected, err := git.GetGitRepoRevision(snapshotPath, branch)
	if err != nil {
		return err
	}
	actual, err := git.GetGitRepoRevision(r.getRemotePath(project), branch)
	if err != nil {
		return err
	}
	if expected != actual {
		return fmt.Errorf("mismatch for branch %s: project at revision %s, snapshot at revision %s", branch, actual, expected)
	}
	return nil
}

// AssertProjectBranchHasAncestor asserts that the specified branch in the project descends
// from the given snapshot.
func (r *RepoHarness) AssertProjectBranchHasAncestor(project repo.Project, branch, snapshotPath string) error {
	ancestor, err := git.GetGitRepoRevision(snapshotPath, project.Revision)
	if err != nil {
		return err
	}
	descendent, err := git.GetGitRepoRevision(r.getRemotePath(project), branch)
	if err != nil {
		return err
	}
	ok, err := git.IsReachable(r.getRemotePath(project), ancestor, descendent)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("branch %s does not descend from snapshot at %s", branch, snapshotPath)
	}
	return nil
}
