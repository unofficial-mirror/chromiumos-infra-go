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
	"reflect"
	"regexp"

	"github.com/otiai10/copy"
	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
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

type RepoHarnessConfig struct {
	// Initialize() will create a test harness with
	// the appropriate remote repos and a local repo.
	// Both remote and local repos will have the appropriate
	// projects created (with initialized git repos inside them).
	Manifest repo.Manifest
}

const (
	readWritePerms = 0666
	dirPerms       = 0777
)

var (
	DefaultCrosHarnessConfig = RepoHarnessConfig{
		Manifest: repo.Manifest{
			Remotes: []repo.Remote{
				{Name: "cros"},
				{Name: "cros-internal"},
			},
		},
	}
)

type RepoHarness struct {
	// Manifest that defines the harness configuration.
	manifest repo.Manifest
	// Root directory of the whole harness setup.
	harnessRoot string
	// Local checkout.
	LocalRepo string
	config    RepoHarnessConfig
}

func (r *RepoHarness) RunCommand(cmd []string, cwd string) error {
	if cwd == "" {
		cwd = r.harnessRoot
	}
	ctx := context.Background()
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

		// Project could already exist due to multiple checkouts. If it does, skip.
		if _, err = os.Stat(projectPath); err == nil {
			continue
		}

		// Create project directory.
		if err = os.MkdirAll(projectPath, dirPerms); err != nil {
			return errors.Annotate(err, "failed to create dir for %s", projectLabel).Err()
		}
		// Initialize bare repo in project directory.
		if err = git.Init(projectPath, true); err != nil {
			return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
		}

		// Make an initial commit so that the "master" branch is not unborn.
		// To do this, we need to make a change in a local repository and push it to the
		// remote.
		// Add the repo as a remote to the corresponding local repo.
		tmpRepo := filepath.Join(r.harnessRoot, "tmp-repo")
		remoteRef := git.RemoteRef{
			Remote: remoteName,
			Ref:    "master",
		}
		errs := []error{
			os.Mkdir(tmpRepo, dirPerms),
			git.Init(tmpRepo, false),
			git.AddRemote(tmpRepo, remoteName, projectPath),
			ioutil.WriteFile(filepath.Join(tmpRepo, ".harnessInit"), []byte(project.Path), readWritePerms),
			git.PushChanges(tmpRepo, "master", "initial commit", false, remoteRef),
			os.RemoveAll(tmpRepo),
		}
		for _, err = range errs {
			if err != nil {
				return errors.Annotate(err, "failed to init git repo for %s", projectLabel).Err()
			}
		}
	}

	// Create local checkout of the project at r.LocalRepo, in case the client would like one.
	// Specifically, we create temporary git repo with the manifest and sync using
	// `repo sync`.
	// TOOD(@jackneus): Will syncing from (and then deleting) a temp repo make subsequent
	// `repo sync`s fail?
	manifestRepo, err := ioutil.TempDir(r.harnessRoot, "manifest-repo")
	defer os.RemoveAll(manifestRepo)
	if err != nil {
		return errors.Annotate(err, "error initializing temp git repo for manifest").Err()
	}
	manifestPath := filepath.Join(manifestRepo, "default.xml")
	errs := []error{
		git.Init(manifestRepo, false),
		r.manifest.Write(manifestPath),
		git.CommitAll(manifestRepo, "commit manifest"),
		r.RunCommand([]string{"repo", "init", "--manifest-url", manifestRepo}, r.LocalRepo),
	}
	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to initialize local checkout").Err()
		}
	}
	err = r.RunCommand([]string{"repo", "sync"}, r.LocalRepo)
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

// TODO(@jackneus): Add support for adding files at particular revisions (or something similar).
// This is needed for projects with multiple checkouts.
func (r *RepoHarness) AddFile(file File) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}

	project, err := r.manifest.GetProjectByPath(file.Project)
	if err != nil {
		return err
	}
	remoteName := project.RemoteName
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
		git.Clone(filepath.Join(r.harnessRoot, remoteName, project.Name), tmpRepo),
		ioutil.WriteFile(filePath, file.Contents, file.Perm),
		git.PushChanges(tmpRepo, "master", "initial commit", false, remoteRef),
		os.RemoveAll(tmpRepo),
	}
	for _, err = range errs {
		if err != nil {
			return errors.Annotate(err, "failed to add file %s to %s", file.Name, projectLabel).Err()
		}
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

func (r *RepoHarness) AddFiles(files []File) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	for _, file := range files {
		err := r.AddFile(file)
		if err != nil {
			return err
		}
	}

	return nil
}

// getRemotePath gets the path to the remote project repo.
func (r *RepoHarness) getRemotePath(project repo.Project) string {
	return filepath.Join(r.harnessRoot, project.RemoteName, project.Name)
}

// unorderedEqual checks that the two arrays contain the same elements, but
// they don't have to be the same order.
func unorderedEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]int)
	for _, v := range a {
		am[v]++
	}
	bm := make(map[string]int)
	for _, v := range b {
		bm[v]++
	}
	return reflect.DeepEqual(am, bm)
}

// AssertProjectBranches asserts that the remote project has the correct branches.
func (r *RepoHarness) AssertProjectBranches(project repo.Project, branches []string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	gitRepo := r.getRemotePath(project)
	actual, err := git.MatchBranchNameWithNamespace(gitRepo, regexp.MustCompile(".*"), regexp.MustCompile("refs/heads/"))
	if err != nil {
		return errors.Annotate(err, "error getting branches").Err()
	}
	if !unorderedEqual(actual, branches) {
		return fmt.Errorf("project branch mismatch. expected: %v got %v", branches, actual)
	}
	return nil
}

// Snapshot recursively copies a directory's contents to a temp dir.
func (r *RepoHarness) Snapshot(path string) (string, error) {
	snapshotDir, err := ioutil.TempDir(r.harnessRoot, "snapshot")
	if err != nil {
		return "", err
	}
	if err = copy.Copy(path, snapshotDir); err != nil {
		return "", err
	}
	return snapshotDir, nil
}

// AssertSameContents checks that there's no difference between two directories.
func (r *RepoHarness) AssertSameContents(path_a, path_b string) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}
	ctx := context.Background()
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := []string{"-rq", path_a, path_b}
	err := CommandRunnerImpl.RunCommand(ctx, &stdoutBuf, &stderrBuf, r.harnessRoot, "diff", cmd...)
	if err != nil {
		if err.Error() == "exit status 1" {
			return fmt.Errorf("files differ")
		} else {
			return fmt.Errorf("error running diff")
		}
	}
	return err
}
