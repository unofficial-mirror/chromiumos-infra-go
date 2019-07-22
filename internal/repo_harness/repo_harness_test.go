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
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
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
			{Path: "foo1/", Name: "foo"},
			{Path: "foo2/", Name: "foo"},
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

func TestInitialize_simple(t *testing.T) {
	harnessConfig := simpleHarnessConfig
	harness := &RepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(&harnessConfig)
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
}

func TestAddFile_simple(t *testing.T) {
	harnessConfig := simpleHarnessConfig
	harness := &RepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(&harnessConfig)
	assert.NilError(t, err)

	for _, file := range simpleFiles {
		assert.NilError(t, harness.AddFile(file))
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

func TestAssertProjectBranches_success(t *testing.T) {
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
	harness := &RepoHarness{}

	// Create a hierachy of files.
	root, err := ioutil.TempDir("", "snapshot_test")
	defer os.RemoveAll(root)
	assert.NilError(t, err)
	bazContents := "foo, bar and baz, oh my!"
	createFooBarBaz(t, root, bazContents)

	// Create snapshot and verify accuracy.
	snapshotDir, err := harness.Snapshot(root)
	assert.NilError(t, err)
	checkFooBarBaz(t, snapshotDir, bazContents)
}
