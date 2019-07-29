// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	rh "go.chromium.org/chromiumos/infra/go/internal/repo_harness"
	"gotest.tools/assert"
)

var testManifest repo.Manifest = repo.Manifest{
	Projects: []repo.Project{
		// Single-checkout project.
		{
			Name: "chromiumos/version",
			Path: "chromiumos/version",
		},
		// Explicitly pinned project.
		{
			Name: "explicit-pinned",
			Path: "explicit-pinned",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "pin"},
			},
			Revision: "refs/heads/explicit-pinned",
		},
		// Implicitly pinned project.
		{
			Name:     "external/implicit-pinned",
			Path:     "src/third_party/implicit-pinned",
			Revision: "refs/heads/implicit-pinned",
		},
		// Multi-checkout project.
		{
			Name:     "chromiumos/multicheckout",
			Path:     "src/third_party/multicheckout-a",
			Revision: "refs/heads/multicheckout-a",
		},
		{
			Name:     "chromiumos/multicheckout",
			Path:     "src/third_party/multicheckout-b",
			Revision: "refs/heads/multicheckout-b",
		},
		// ToT project.
		{
			Name: "tot",
			Path: "tot",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "tot"},
			},
		},
	},
	Remotes: []repo.Remote{
		{Name: "cros"},
	},
	Default: repo.Default{
		RemoteName: "cros",
		Revision:   "refs/heads/master",
	},
}

func testInitialize(t *testing.T, config *CrosRepoHarnessConfig) {
	harness := &CrosRepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(config)
	assert.NilError(t, err)
}

func TestInitializeSimple(t *testing.T) {
	testInitialize(t, &DefaultCrosHarnessConfig)
}

func TestInitializeAllProjectTypes(t *testing.T) {
	config := &CrosRepoHarnessConfig{
		Manifest:       testManifest,
		VersionProject: "chromiumos/version",
	}
	testInitialize(t, config)
}

func TestInitialize_badVersionProject(t *testing.T) {
	config := &CrosRepoHarnessConfig{
		Manifest:       testManifest,
		VersionProject: "bogus",
	}
	harness := &CrosRepoHarness{}
	defer harness.Teardown()
	err := harness.Initialize(config)
	assert.ErrorContains(t, err, "does not exist")
}

func TestSetVersion(t *testing.T) {
	config := DefaultCrosHarnessConfig
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(&config)
	assert.NilError(t, err)

	versionFileName := "version.sh"
	version := repo.VersionInfo{
		ChromeBranch:      1,
		BuildNumber:       2,
		BranchBuildNumber: 3,
		PatchNumber:       4,
		VersionFile:       versionFileName,
	}
	assert.NilError(t, r.SetVersion("", version))

	// Test that version file was written correctly.
	harnessRoot := r.harness.HarnessRoot()
	tmpRepo, err := ioutil.TempDir(harnessRoot, "test_harness_test")
	assert.NilError(t, err)
	versionProject := rh.GetRemoteProject(*r.versionProject)
	versionProjectPath := filepath.Join(harnessRoot, versionProject.RemoteName, versionProject.ProjectName)

	assert.NilError(t, git.Clone(versionProjectPath, tmpRepo))
	contents, err := ioutil.ReadFile(filepath.Join(tmpRepo, versionFileName))
	assert.NilError(t, err)
	vinfo, err := repo.ParseVersionInfo(contents)
	assert.NilError(t, err)
	assert.Assert(t, repo.VersionsEqual(vinfo, version))
}

func TestTakeSnapshot(t *testing.T) {
	config := DefaultCrosHarnessConfig
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(&config)
	assert.NilError(t, err)

	assert.NilError(t, r.TakeSnapshot())

	// Check that snapshots exist.
	for _, remote := range config.Manifest.Remotes {
		snapshotPath, ok := r.recentRemoteSnapshots[remote.Name]
		assert.Assert(t, ok)
		_, err := os.Stat(snapshotPath)
		assert.NilError(t, err)
	}
}
