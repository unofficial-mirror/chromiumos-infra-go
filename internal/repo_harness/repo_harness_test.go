// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo_harness

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/chromiumos/infra/go/internal/test_util"
	"gotest.tools/assert"
)

var simpleHarnessConfig = RepoHarnessConfig{
	Manifest: repo.Manifest{
		Remotes: []repo.Remote{
			{Name: "cros"},
			{Name: "cros-internal"},
		},
		Default: repo.Default{
			RemoteName: "cros",
			Revision:   "refs/heads/master",
		},
		Projects: []repo.Project{
			{Path: "foo1/", Name: "foo", Revision: "refs/heads/foo1"},
			{Path: "foo2/", Name: "foo", Revision: "refs/heads/foo2"},
			{Path: "bar/", Name: "bar"},
			{Path: "baz/", Name: "baz", RemoteName: "cros-internal"},
		},
	},
}
var simpleFiles = []File{
	{Project: "foo1/", Name: "README", Contents: []byte("foo1")},
	{Project: "foo2/", Name: "ALSO_README", Contents: []byte("foo2")},
	{Project: "bar/", Name: "README", Contents: []byte("bar")},
	{Project: "baz/", Name: "SECRET", Contents: []byte("internal only")},
}
var multilevelProjectHarnessConfig = RepoHarnessConfig{
	Manifest: repo.Manifest{
		Remotes: []repo.Remote{
			{Name: "remote"},
		},
		Default: repo.Default{
			RemoteName: "remote",
			Revision:   "refs/heads/master",
		},
		Projects: []repo.Project{
			{Path: "src/foo/bar", Name: "foo/bar"},
		},
	},
}

func testInitialize(t *testing.T, config *RepoHarnessConfig) {
	harnessConfig := config
	harness := &RepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(harnessConfig)
	assert.NilError(t, err)

	// Check that snapshots/ dir was created.
	_, err = os.Stat(filepath.Join(harness.harnessRoot, "snapshots"))
	assert.NilError(t, err)

	// Check that all local repos were created.
	for _, project := range harnessConfig.Manifest.Projects {
		_, err := os.Stat(filepath.Join(harness.LocalRepo, project.Path))
		assert.NilError(t, err)
	}

	// Check that all remotes were created.
	for _, remote := range harnessConfig.Manifest.Remotes {
		_, err := os.Stat(filepath.Join(harness.harnessRoot, remote.Name))
		assert.NilError(t, err)
	}

	// Check that appropraite projects were created and initialized.
	for _, project := range harnessConfig.Manifest.Projects {
		projectPath := filepath.Join(harness.harnessRoot, project.RemoteName, project.Name)
		_, err := os.Stat(projectPath)
		assert.NilError(t, err)
		branches, err := git.MatchBranchName(projectPath, regexp.MustCompile("master"))
		assert.NilError(t, err)
		assert.Assert(t, test_util.UnorderedContains(branches, []string{git.NormalizeRef("master")}))

		// If project has revision set, check that that branch was create too.
		if project.Revision != "" && project.Revision != "master" {
			revisionBranch := project.Revision
			branches, err := git.MatchBranchName(projectPath, regexp.MustCompile(revisionBranch))
			assert.NilError(t, err)
			assert.Assert(t, test_util.UnorderedContains(branches, []string{revisionBranch}))
		}
	}
}

func TestInitialize_simple(t *testing.T) {
	testInitialize(t, &simpleHarnessConfig)
}

// Test that a project with a multi-level name (e.g. foo/bar) is properly
// created in the appropriate remote.
func TestInitialize_multilevelProject(t *testing.T) {
	testInitialize(t, &multilevelProjectHarnessConfig)
}

func TestInitialize_badRevision(t *testing.T) {
	harnessConfig := RepoHarnessConfig{
		Manifest: repo.Manifest{
			Projects: []repo.Project{
				{Name: "foo",
					Revision:   "deadbeef",
					RemoteName: "cros"},
			},
			Remotes: []repo.Remote{
				{Name: "cros"},
			},
		},
	}
	harness := &RepoHarness{}
	defer harness.Teardown()
	assert.ErrorContains(t, harness.Initialize(&harnessConfig), "refs/heads")
}

func TestCreateRemoteRef(t *testing.T) {
	root, err := ioutil.TempDir("", "create_remote_ref_test")
	defer os.RemoveAll(root)
	assert.NilError(t, err)

	harness := &RepoHarness{
		manifest: repo.Manifest{
			Remotes: []repo.Remote{
				{Name: "cros"},
			},
			Projects: []repo.Project{
				{Path: "foo/", Name: "foo", RemoteName: "cros"},
			},
		},
		harnessRoot: root,
	}
	// Set up remote.
	remotePath := filepath.Join(harness.harnessRoot, harness.manifest.Remotes[0].Name)
	assert.NilError(t, os.Mkdir(remotePath, dirPerms))
	// Set up remote project.
	project := harness.manifest.Projects[0]
	remoteProjectPath := filepath.Join(remotePath, project.Name)
	assert.NilError(t, os.Mkdir(remoteProjectPath, dirPerms))
	assert.NilError(t, git.Init(remoteProjectPath, false))

	// Make initial commit.
	_, err = git.RunGit(remoteProjectPath, []string{"commit", "-m", "init", "--allow-empty"})
	assert.NilError(t, err)
	output, err := git.RunGit(remoteProjectPath, []string{"rev-parse", "HEAD"})
	assert.NilError(t, err)
	commit := strings.TrimSpace(output.Stdout)
	assert.NilError(t, harness.CreateRemoteRef(project, "ref1", commit))
	assert.NilError(t, harness.CreateRemoteRef(project, "ref2", ""))

	output, err = git.RunGit(remoteProjectPath, []string{"show-ref"})
	refs := []string{}
	for _, line := range strings.Split(output.Stdout, "\n") {
		if line == "" {
			continue
		}
		refs = append(refs, strings.Fields(line)[1])
	}
	assert.Assert(t, test_util.UnorderedContains(refs, []string{"refs/heads/ref1", "refs/heads/ref2"}))
}

func TestAddFile_simple(t *testing.T) {
	harnessConfig := simpleHarnessConfig
	harness := &RepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(&harnessConfig)
	assert.NilError(t, err)

	for _, file := range simpleFiles {
		_, err = harness.AddFile(file)
		assert.NilError(t, err)
	}

	// Check that all files were added to remotes.
	for _, file := range simpleFiles {
		project, err := harness.manifest.GetProjectByPath(file.Project)
		assert.NilError(t, err)
		remoteName := project.RemoteName

		tmpDir, err := ioutil.TempDir(harness.harnessRoot, "tmp-clone-dir")

		err = git.Clone(filepath.Join(harness.harnessRoot, remoteName, project.Name), tmpDir)
		assert.NilError(t, err)

		// Check that file exists.
		filePath := filepath.Join(tmpDir, file.Name)
		_, err = os.Stat(filePath)
		assert.NilError(t, err)
		// Check file contents.
		fileContents, err := ioutil.ReadFile(filePath)
		assert.NilError(t, err)
		assert.Assert(t, reflect.DeepEqual(file.Contents, fileContents))

		os.RemoveAll(tmpDir)
	}
}

func TestAddFiles_simple(t *testing.T) {
	harnessConfig := simpleHarnessConfig
	harness := &RepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(&harnessConfig)
	assert.NilError(t, err)

	assert.NilError(t, harness.AddFiles(simpleFiles))

	// Check that all files were added to remotes.
	for _, file := range simpleFiles {
		project, err := harness.manifest.GetProjectByPath(file.Project)
		assert.NilError(t, err)
		remoteName := project.RemoteName

		tmpDir, err := ioutil.TempDir(harness.harnessRoot, "tmp-clone-dir")

		err = git.Clone(filepath.Join(harness.harnessRoot, remoteName, project.Name), tmpDir)
		assert.NilError(t, err)

		// Check that file exists.
		filePath := filepath.Join(tmpDir, file.Name)
		_, err = os.Stat(filePath)
		assert.NilError(t, err)
		// Check file contents.
		fileContents, err := ioutil.ReadFile(filePath)
		assert.NilError(t, err)
		assert.Assert(t, reflect.DeepEqual(file.Contents, fileContents))

		os.RemoveAll(tmpDir)
	}
}

func TestTeardown(t *testing.T) {
	// Pretend that harness has been initialized and harness root has been created.
	tmpDir := "harness_root"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)

	harness := RepoHarness{
		harnessRoot: tmpDir,
	}
	// Sanity check: harness root exists.
	_, err = os.Stat(tmpDir)
	assert.NilError(t, err)
	// Perform teardown.
	assert.NilError(t, harness.Teardown())
	_, err = os.Stat(tmpDir)
	// Root no longer exists.
	assert.Assert(t, os.IsNotExist(err))
	assert.Equal(t, harness.harnessRoot, "")
}

func TestGetRemotePath(t *testing.T) {
	harnessConfig := simpleHarnessConfig
	harness := &RepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(&harnessConfig)
	assert.NilError(t, err)

	project := harness.manifest.Projects[0]
	expectedPath := filepath.Join(harness.harnessRoot, project.RemoteName, project.Name)
	assert.Equal(t, harness.getRemotePath(project), expectedPath)
}

func TestAssertProjectBranches(t *testing.T) {
	harness := &RepoHarness{
		harnessRoot: "foo",
	}
	project := repo.Project{
		RemoteName: "bar",
		Name:       "baz",
	}
	projectPath := "foo/bar/baz"

	branches := []string{"master", "branch"}
	stdout := ""
	for _, branch := range branches {
		stdout += fmt.Sprintf("aaa refs/heads/%s\n", branch)
	}

	git.CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "show-ref"},
		ExpectedDir: projectPath,
		Stdout:      stdout,
	}

	assert.NilError(t, harness.AssertProjectBranches(project, branches))
	assert.ErrorContains(t, harness.AssertProjectBranches(project, []string{"bad"}), "mismatch")

	// Set command runner back to the real one. Most tests in this package do not mock git.
	git.CommandRunnerImpl = cmd.RealCommandRunner{}
}

func TestAssertProjectBranchesExact(t *testing.T) {
	harness := &RepoHarness{
		harnessRoot: "foo",
	}
	project := repo.Project{
		RemoteName: "bar",
		Name:       "baz",
	}
	projectPath := "foo/bar/baz"

	branches := []string{"master", "branch"}
	stdout := ""
	for _, branch := range branches {
		stdout += fmt.Sprintf("aaa refs/heads/%s\n", branch)
	}

	git.CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "show-ref"},
		ExpectedDir: projectPath,
		Stdout:      stdout,
	}

	assert.NilError(t, harness.AssertProjectBranchesExact(project, branches))
	assert.ErrorContains(t, harness.AssertProjectBranchesExact(project, append(branches, "extra")), "mismatch")

	// Set command runner back to the real one. Most tests in this package do not mock git.
	git.CommandRunnerImpl = cmd.RealCommandRunner{}
}

// createFooBarBaz creates foo bar baz file structure, the greatest file structure on earth
func createFooBarBaz(t *testing.T, root, bazContents string) {
	assert.NilError(t, os.Mkdir(filepath.Join(root, "foo"), 0755))
	assert.NilError(t, os.Mkdir(filepath.Join(root, "foo", "bar"), 0755))
	assert.NilError(t, ioutil.WriteFile(filepath.Join(root, "foo", "bar", "baz"), []byte(bazContents), 0666))
}

// checkFooBarBaz checks the foo bar baz file structure, the greatest file structure on earth
func checkFooBarBaz(t *testing.T, root, bazContents string) {
	_, err := os.Stat(filepath.Join(root, "foo"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(root, "foo", "bar"))
	assert.NilError(t, err)
	snapshotBazPath := filepath.Join(root, "foo", "bar", "baz")
	_, err = os.Stat(snapshotBazPath)
	assert.NilError(t, err)
	// Check contents of bar/baz.
	contents, err := ioutil.ReadFile(snapshotBazPath)
	assert.NilError(t, err)
	assert.Equal(t, string(contents), bazContents)
}

func TestSnapshot(t *testing.T) {
	root, err := ioutil.TempDir("", "assert_test")
	assert.NilError(t, err)
	defer os.RemoveAll(root)
	harness := &RepoHarness{
		harnessRoot: root,
	}
	assert.NilError(t, os.Mkdir(filepath.Join(harness.harnessRoot, "snapshots"), 0777))

	// Create a hierachy of files.
	fooRoot, err := ioutil.TempDir(harness.harnessRoot, "snapshot_test")
	assert.NilError(t, err)
	bazContents := "foo, bar and baz, oh my!"
	createFooBarBaz(t, fooRoot, bazContents)

	// Create snapshot and verify accuracy.
	snapshotDir, err := harness.Snapshot(fooRoot)
	assert.NilError(t, err)
	checkFooBarBaz(t, snapshotDir, bazContents)
}

func TestAssertProjectBranchEqual(t *testing.T) {
	root, err := ioutil.TempDir("", "assert_test")
	assert.NilError(t, err)
	defer os.RemoveAll(root)
	harness := &RepoHarness{
		harnessRoot: root,
	}

	local, err := ioutil.TempDir(harness.harnessRoot, "")
	assert.NilError(t, err)
	remote, err := ioutil.TempDir(harness.harnessRoot, "")
	assert.NilError(t, err)

	project := repo.Project{
		Name: filepath.Base(local),
	}

	// Initialize remote repo and make a commit.
	assert.NilError(t, git.Init(remote, false))
	assert.NilError(t, ioutil.WriteFile(filepath.Join(remote, "foo"), []byte("foo"), 0644))
	_, err = git.CommitAll(remote, "init commit")
	assert.NilError(t, err)
	// Clone remote so that we have two identical repos.
	assert.NilError(t, git.Clone(remote, local))
	fmt.Printf("%s %s\n", remote, local)
	assert.NilError(t, harness.AssertProjectBranchEqual(project, "master", remote))
	// Now, make commit to local.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(local, "bar"), []byte("bar"), 0644))
	_, err = git.CommitAll(local, "addl commit")
	assert.NilError(t, err)
	assert.ErrorContains(t, harness.AssertProjectBranchEqual(project, "master", remote), "mismatch")
}

func TestAssertProjectBranchHasAncestor(t *testing.T) {
	root, err := ioutil.TempDir("", "assert_test")
	assert.NilError(t, err)
	defer os.RemoveAll(root)
	harness := &RepoHarness{
		harnessRoot: root,
	}

	local, err := ioutil.TempDir(harness.harnessRoot, "")
	assert.NilError(t, err)
	remote, err := ioutil.TempDir(harness.harnessRoot, "")
	assert.NilError(t, err)

	project := repo.Project{
		Name: filepath.Base(local),
	}

	// Initialize remote repo and make a commit.
	assert.NilError(t, git.Init(remote, false))
	assert.NilError(t, ioutil.WriteFile(filepath.Join(remote, "foo"), []byte("foo"), 0644))
	_, err = git.CommitAll(remote, "init commit")
	assert.NilError(t, err)
	// Clone remote so that we have two identical repos.
	assert.NilError(t, git.Clone(remote, local))
	fmt.Printf("%s %s\n", remote, local)
	assert.NilError(t, harness.AssertProjectBranchHasAncestor(project, "master", remote))

	// Now, make commit to local. We should still be good.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(local, "bar"), []byte("bar"), 0644))
	_, err = git.CommitAll(local, "addl commit")
	assert.NilError(t, err)
	assert.NilError(t, harness.AssertProjectBranchHasAncestor(project, "master", remote))

	// But if we make a commit to remote, our local repo will no longer descend from it.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(remote, "baz"), []byte("baz"), 0644))
	_, err = git.CommitAll(remote, "addl commit")
	assert.NilError(t, err)
	assert.ErrorContains(t, harness.AssertProjectBranchHasAncestor(project, "master", remote), "does not descend")
}
