// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo_util

import (
	"gotest.tools/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
)

func TestInitialize_success(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	manifestUrl := "https://chromium.googlesource.com/chromiumos/manifest/foo.xml"
	commandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"repo", "init", "--manifest-url", manifestUrl},
		ExpectedDir: tmpDir,
	}
	repo, err := Initialize(tmpDir, manifestUrl, "repo")
	assert.NilError(t, err)
	assert.Equal(t, repo.Root, tmpDir)
}

func TestInitialize_repoexists(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	// .repo directory already exists
	assert.NilError(t, os.Mkdir(filepath.Join(tmpDir, ".repo"), 0775))

	manifestUrl := "https://chromium.googlesource.com/chromiumos/manifest/foo.xml"
	_, err = Initialize(tmpDir, manifestUrl, "repo")
	assert.ErrorContains(t, err, "existing")
}

func TestInitialize_repo_failure(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	manifestUrl := "https://chromium.googlesource.com/chromiumos/manifest/foo.xml"
	commandRunnerImpl = cmd.FakeCommandRunner{
		FailCommand: true,
	}
	_, err = Initialize(tmpDir, manifestUrl, "repo")
	assert.ErrorContains(t, err, "")
}

func TestInitialize_missingManifestUrl(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	manifestUrl := ""
	commandRunnerImpl = cmd.FakeCommandRunner{
		FailCommand: true,
	}
	_, err = Initialize(tmpDir, manifestUrl, "repo")
	assert.ErrorContains(t, err, "url")
}

func TestSyncToFile_success(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	// Create .repo folder
	assert.NilError(t, os.Mkdir(filepath.Join(tmpDir, ".repo"), 0775))
	// Create manifest file
	file, err := ioutil.TempFile(tmpDir, "foo*.xml")
	assert.NilError(t, err)

	manifestFile := file.Name()
	commandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"repo", "sync", "--manifest-name", manifestFile},
	}
	testRepo := &Repository{tmpDir}
	err = testRepo.SyncToFile(manifestFile, "repo")
	assert.NilError(t, err)
}

func TestSyncToFile_manifest_missing(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	// Create .repo folder
	assert.NilError(t, os.Mkdir(filepath.Join(tmpDir, ".repo"), 0775))

	commandRunnerImpl = cmd.FakeCommandRunner{}
	testRepo := &Repository{tmpDir}
	err = testRepo.SyncToFile("foo", "repo")
	assert.ErrorContains(t, err, "exist")
}

func TestSyncToFile_repo_no_init(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	commandRunnerImpl = cmd.FakeCommandRunner{}
	testRepo := &Repository{tmpDir}
	err = testRepo.SyncToFile("foo", "repo")
	assert.ErrorContains(t, err, "init")
}

func TestManifest(t *testing.T) {
	manifestData :=
		`<?xml version="1.0" encoding="UTF-8"?>` +
			`<manifest>` +
			`  <project path="src/foo" name="foo"/>` +
			`  <project path="src/bar" name="bar"/>` +
			`  <project path="src/baz" name="baz"/>` +
			`</manifest>`

	// Set up temporary Root
	tmpDir, err := ioutil.TempDir("", "repotest_tmp_dir")
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	testRepo := &Repository{tmpDir}
	commandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: tmpDir,
		Stdout:      manifestData,
	}
	expectedManifest := repo.Manifest{
		Projects: []repo.Project{
			{Path: "src/foo", Name: "foo"},
			{Path: "src/bar", Name: "bar"},
			{Path: "src/baz", Name: "baz"},
		},
	}

	manifest, err := testRepo.Manifest("repo")
	assert.NilError(t, err)
	assert.DeepEqual(t, manifest, expectedManifest)
}
