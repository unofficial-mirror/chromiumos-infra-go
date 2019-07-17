// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo_harness

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

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
