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

type File struct {
	Name     string
	Contents []byte
	Perm     os.FileMode
}

const (
	readWritePerms = 0666
	dirPerms       = 0777
)

// Identifies a remote project.
type RemoteProject struct {
	RemoteName  string
	ProjectName string
}

func GetRemoteProject(project repo.Project) RemoteProject {
	return RemoteProject{
		RemoteName:  project.RemoteName,
		ProjectName: project.Name,
	}
}

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
	// Git repo that manifest (from config) is stored in.
	manifestRepo string
}

func (r *RepoHarness) Manifest() repo.Manifest {
	return r.manifest
}

func (r *RepoHarness) HarnessRoot() string {
	return r.harnessRoot
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
	// Protect client, because we really don't want repo to try and hit chromium.googlesource.com.
	// Unless client has explicitly messed with the manifest to make it work, this won't work
	// and will fail fairly slowly.
	if len(config.Manifest.Remotes) == 0 {
		return fmt.Errorf("no remotes specified in config manifest")
	}
	if config.Manifest.Default.RemoteName == "" {
		config.Manifest.Default.RemoteName = config.Manifest.Remotes[0].Name
	}
	if config.Manifest.Default.Revision == "" {
		config.Manifest.Default.Revision = "refs/heads/master"
	}

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
			if err = r.CreateRemoteRef(GetRemoteProject(project), "master", ""); err != nil {
				return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
			}
		}
		// If revision is set, create that branch too.
		if project.Revision != "" && !strings.HasPrefix(project.Revision, "refs/heads/") {
			return fmt.Errorf("revisions must be of the form refs/heads/<branch>")
		}

		revision := git.StripRefs(project.Revision)
		if revision != "" && revision != "master" {
			// Creating the revision ref from a fresh repo/commit and not from refs/heads/master is
			// kind of nice because it removes some false positives from AssertCrosBranchFromManifest
			// -- if a multicheckout branch is created from refs/heads/master instead of its set
			// revision, the assert would still pass if the revision itself descends from refs/heads/master.
			if err = r.CreateRemoteRef(GetRemoteProject(project), revision, ""); err != nil {
				return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
			}
		}
	}

	// Create local checkout of the project at r.LocalRepo, in case the client would like one.
	// Specifically, we create a git repo with the manifest and sync using
	// `repo sync`.
	r.manifestRepo = filepath.Join(r.harnessRoot, "manifest-repo")
	if err = os.Mkdir(r.manifestRepo, dirPerms); err != nil {
		return errors.Annotate(err, "error initializing git repo for manifest").Err()
	}
	manifestPath := filepath.Join(r.manifestRepo, "default.xml")
	errs := []error{
		git.Init(r.manifestRepo, false),
		r.manifest.Write(manifestPath),
	}
	_, err = git.CommitAll(r.manifestRepo, "commit manifest")
	errs = append(errs,
		err,
		r.runCommand([]string{"repo", "init", "--manifest-url", r.manifestRepo}, r.LocalRepo),
	)

	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to initialize local checkout").Err()
		}
	}
	err = r.SyncLocalCheckout()
	if err != nil {
		return err
	}

	// Certain tools can't read from a project unless refs/heads/master exists
	// ex: repo init --manifest-url
	err = r.runCommand([]string{"repo", "start", "master", "--all"}, r.LocalRepo)

	return err
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

// SyncLocalCheckout syncs the harness' local checkout to the manifest
// specified at initialization.
func (r *RepoHarness) SyncLocalCheckout() error {
	if err := r.assertInitialized(); err != nil {
		return err
	}

	err := r.runCommand([]string{"repo", "sync"}, r.LocalRepo)
	if err != nil {
		return errors.Annotate(err, "failed to sync local checkout").Err()
	}

	return nil
}

// CreateRemoteRef creates a remote ref for a specific project.
// Otherwise, a temporary local checkout will be created and an empty commit
// will be used to create the remote ref.
func (r *RepoHarness) CreateRemoteRef(project RemoteProject, ref string, commit string) error {
	projectLabel := fmt.Sprintf("%s/%s", project.RemoteName, project.ProjectName)
	remoteProjectPath := r.GetRemotePath(project)

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

	remoteExists, err := git.RemoteHasBranch(repoPath, remoteRef.Remote, remoteRef.Ref)
	if err != nil {
		return errors.Annotate(err, "failed to ls-remote remote %s", remoteRef.Remote).Err()
	}
	if remoteExists {
		return fmt.Errorf("remote ref %s already exists", ref)
	}

	if err := git.PushRef(repoPath, commit, false, remoteRef); err != nil {
		return errors.Annotate(err, "failed to add remote ref %s %s:%s", projectLabel, commit, remoteRef.Ref).Err()
	}
	return nil
}

// AddFile adds a file to the specified branch in the specified remote project.
// Returns the sha1 of the commit that adds the file.
func (r *RepoHarness) AddFile(project RemoteProject, branch string, file File) (string, error) {
	return r.AddFiles(project, branch, []File{file})
}

// AddFiles adds files to the specified branch in the specified remote project.
// Returns a map with the sha1's of the commits.
func (r *RepoHarness) AddFiles(project RemoteProject, branch string, files []File) (string, error) {
	if err := r.assertInitialized(); err != nil {
		return "", err
	}

	projectLabel := fmt.Sprintf("%s", project.ProjectName)

	// Populate project in specified remote with files. Because the remote repository is bare,
	// we need to write/commit the files locally and then push them to the remote.
	// We do this using a temp checkout of the appropriate remote.
	tmpRepo, err := ioutil.TempDir(r.harnessRoot, "tmp-repo")
	defer os.RemoveAll(tmpRepo)

	projectPath := r.GetRemotePath(project)
	remoteRef := git.RemoteRef{
		Remote: project.RemoteName,
		Ref:    branch,
	}

	errs := []error{
		err,
		git.Init(tmpRepo, false),
		git.AddRemote(tmpRepo, project.RemoteName, projectPath),
		git.CreateTrackingBranch(tmpRepo, "tmp", remoteRef),
	}

	for _, file := range files {
		filePath := filepath.Join(tmpRepo, file.Name)
		// Set file perms to default value if not specified.
		if file.Perm == 0 {
			file.Perm = readWritePerms
		}

		errs = append(errs,
			os.MkdirAll(filepath.Dir(filePath), dirPerms),
			ioutil.WriteFile(filePath, file.Contents, file.Perm))
	}

	commit, err := git.CommitAll(tmpRepo, "add files")
	errs = append(errs, err, git.PushRef(tmpRepo, "tmp", false, remoteRef))

	for _, err = range errs {
		if err != nil {
			return "", errors.Annotate(err, "failed to add files to %s", projectLabel).Err()
		}
	}

	return commit, nil
}

// ReadFile reads a file from a remote.
func (r *RepoHarness) ReadFile(project RemoteProject, branch, filePath string) ([]byte, error) {
	if err := r.assertInitialized(); err != nil {
		return []byte{}, err
	}
	tmpRepo, err := ioutil.TempDir(r.harnessRoot, "tmp-repo")
	defer os.RemoveAll(tmpRepo)

	remotePath := r.GetRemotePath(project)
	remoteRef := git.RemoteRef{
		Remote: "remote",
		Ref:    branch,
	}
	remoteBranch := fmt.Sprintf("%s/%s", remoteRef.Remote, git.StripRefs(remoteRef.Ref))
	// Checkout just the file we need.
	errs := []error{
		git.Init(tmpRepo, false),
		git.AddRemote(tmpRepo, remoteRef.Remote, remotePath),
		git.RunGitIgnoreOutput(tmpRepo, []string{"fetch", remoteRef.Remote, "--depth", "1"}),
		git.RunGitIgnoreOutput(tmpRepo, []string{"checkout", remoteBranch, "--", filePath}),
	}
	contents, err := ioutil.ReadFile(filepath.Join(tmpRepo, filePath))
	errs = append(errs, err)

	for _, err = range errs {
		if err != nil {
			return []byte{}, errors.Annotate(err, "failed to read file %s from %s/%s", filePath, project.RemoteName, branch).Err()
		}
	}

	return contents, nil
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

// GetRemotePath returns the path to the remote project repo.
func (r *RepoHarness) GetRemotePath(project RemoteProject) string {
	return filepath.Join(r.harnessRoot, project.RemoteName, project.ProjectName)
}

// GetLocalPath returns the path to the local project repo.
func (r *RepoHarness) GetLocalPath(project repo.Project) string {
	return filepath.Join(r.LocalRepo, project.Path)
}

// AssertProjectBranches asserts that the remote project has the correct branches.
func (r *RepoHarness) AssertProjectBranches(project RemoteProject, branches []string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	gitRepo := r.GetRemotePath(project)
	return test_util.AssertGitBranches(gitRepo, branches)
}

// AssertProjectBranchesExact asserts that the remote project has only the correct branches.
func (r *RepoHarness) AssertProjectBranchesExact(project RemoteProject, branches []string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	gitRepo := r.GetRemotePath(project)
	return test_util.AssertGitBranchesExact(gitRepo, branches)
}

// AssertProjectBranchesMissing asserts that the remote project does not have the specified branches.
func (r *RepoHarness) AssertProjectBranchesMissing(project RemoteProject, branches []string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	gitRepo := r.GetRemotePath(project)
	assert := test_util.AssertGitBranchesExact(gitRepo, branches)
	if assert != nil && strings.Contains(assert.Error(), "mismatch") {
		return nil
	}
	return fmt.Errorf("project branch mismatch. some of %v existed.", branches)
}

// AssertProjectBranchEqual asserts that the specified branch in the project matches
// the corresponding branch in the given snapshot.
func (r *RepoHarness) AssertProjectBranchEqual(project RemoteProject, branch, snapshotPath string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	expected, err := git.GetGitRepoRevision(snapshotPath, branch)
	if err != nil {
		return err
	}
	actual, err := git.GetGitRepoRevision(r.GetRemotePath(project), branch)
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
func (r *RepoHarness) AssertProjectBranchHasAncestor(project RemoteProject, branch, snapshotPath, snapshotBranch string) error {
	ancestor, err := git.GetGitRepoRevision(snapshotPath, snapshotBranch)
	if err != nil {
		return err
	}
	descendent, err := git.GetGitRepoRevision(r.GetRemotePath(project), branch)
	if err != nil {
		return err
	}

	ok, err := git.IsReachable(r.GetRemotePath(project), ancestor, descendent)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("branch %s does not descend from snapshot at %s", branch, snapshotPath)
	}
	return nil
}
