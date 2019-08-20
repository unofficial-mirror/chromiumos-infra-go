// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test

import (
	"encoding/xml"
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
	harnessRoot := r.Harness.HarnessRoot()
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

func TestAssertCrosBranches_true(t *testing.T) {
	manifest := testManifest
	config := &CrosRepoHarnessConfig{
		Manifest:       manifest,
		VersionProject: "chromiumos/version",
	}
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(config)
	assert.NilError(t, err)

	crosBranchName := "mybranch"
	// Set up CrOS branch.
	// Create appropriate refs for non-pinned/tot projects.
	// chromiumos/project
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[0]), crosBranchName, ""))
	// chromiumos/multicheckout-a
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[3]), crosBranchName+"-multicheckout-a", ""))
	// chromiumos/multicheckout-b
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[4]), crosBranchName+"-multicheckout-b", ""))

	assert.NilError(t, r.AssertCrosBranches([]string{crosBranchName}))

	// Also test AssertCrosBranchesMissing
	assert.NilError(t, r.AssertCrosBranchesMissing([]string{"bad"}))
	assert.ErrorContains(t, r.AssertCrosBranchesMissing([]string{crosBranchName}), "mismatch")
}

func TestAssertCrosBranches_false(t *testing.T) {
	manifest := testManifest
	config := &CrosRepoHarnessConfig{
		Manifest:       manifest,
		VersionProject: "chromiumos/version",
	}
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(config)
	assert.NilError(t, err)

	crosBranchName := "mybranch"
	// Set up CrOS branch.
	// Create appropriate refs for non-pinned/tot projects.
	// chromiumos/project
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[0]), crosBranchName, ""))
	// chromiumos/multicheckout-a
	// Don't add suffix to branch name (this will result in an invalid CrOS branch).
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[3]), crosBranchName, ""))
	assert.ErrorContains(t, r.AssertCrosBranches([]string{crosBranchName}), "mismatch")
}

func TestAssertCrosBranchFromManifest_true(t *testing.T) {
	manifest := testManifest
	config := &CrosRepoHarnessConfig{
		Manifest:       manifest,
		VersionProject: "chromiumos/version",
	}
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(config)
	assert.NilError(t, err)
	assert.NilError(t, r.TakeSnapshot())

	crosBranchName := "mybranch"
	// Set up CrOS branch. We create the new refs from the corresponding master refs so
	// that the new branch WILL descend from the manifest.
	// Create appropriate refs for non-pinned/tot projects.
	// chromiumos/project
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[0]), crosBranchName, "refs/heads/master"))
	// chromiumos/multicheckout-a
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[3]), crosBranchName+"-multicheckout-a", "refs/heads/multicheckout-a"))
	// chromiumos/multicheckout-b
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[4]), crosBranchName+"-multicheckout-b", "refs/heads/multicheckout-b"))

	assert.NilError(t, r.AssertCrosBranchFromManifest(manifest, crosBranchName, ""))
}

func TestAssertCrosBranchFromManifest_false(t *testing.T) {
	manifest := testManifest
	config := &CrosRepoHarnessConfig{
		Manifest:       manifest,
		VersionProject: "chromiumos/version",
	}
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(config)
	assert.NilError(t, err)
	assert.NilError(t, r.TakeSnapshot())

	crosBranchName := "mybranch"

	// Set up CrOS branch. We create the new refs from the corresponding master refs so
	// that the new branch will NOT descend from the manifest.
	// Specifically, we create the multicheckout branches from refs/heads/master instead of
	// their set revisions.

	// chromiumos/project
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[0]), crosBranchName, "refs/heads/master"))
	// chromiumos/multicheckout-a
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[3]), crosBranchName+"-multicheckout-a", "refs/heads/master"))
	// chromiumos/multicheckout-b
	assert.NilError(t, r.Harness.CreateRemoteRef(rh.GetRemoteProject(manifest.Projects[4]), crosBranchName+"-multicheckout-b", "refs/heads/master"))

	assert.ErrorContains(t, r.AssertCrosBranchFromManifest(manifest, crosBranchName, ""), "does not descend")
}

func TestAssertCrosVersion(t *testing.T) {
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

	assert.NilError(t, r.AssertCrosVersion("master", version))
	// Wrong version.
	version.ChromeBranch = 5
	assert.ErrorContains(t, r.AssertCrosVersion("master", version), "version mismatch")
	// Wrong branch.
	version.ChromeBranch = 5
	assert.Assert(t, r.AssertCrosVersion("branch", version) != nil)
}

func TestAssertNoDefaultRevisions(t *testing.T) {
	manifest := repo.Manifest{
		Default: repo.Default{},
		Remotes: []repo.Remote{
			{Name: "remote"},
		},
	}
	assert.NilError(t, AssertNoDefaultRevisions(manifest))

	manifest = repo.Manifest{
		Default: repo.Default{
			Revision: "foo",
		},
		Remotes: []repo.Remote{
			{Name: "remote"},
		},
	}
	assert.ErrorContains(t, AssertNoDefaultRevisions(manifest), "<default>")

	manifest = repo.Manifest{
		Default: repo.Default{},
		Remotes: []repo.Remote{
			{Name: "remote", Revision: "foo"},
		},
	}
	assert.ErrorContains(t, AssertNoDefaultRevisions(manifest), "<remote>")
}

func TestAssertProjectRevisionsMatchBranch(t *testing.T) {
	config := CrosRepoHarnessConfig{
		Manifest:       testManifest,
		VersionProject: "chromiumos/version",
	}
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(&config)
	assert.NilError(t, err)

	manifest := r.Harness.Manifest()
	// Deep copy projects so that we can change manifest without changing r.Harness.manifest
	manifest.Projects = append([]repo.Project(nil), manifest.Projects...)

	// To avoid all the work of actually branching, just switch the revisions on
	// pinned projects to be SHA-1's instead of 'refs/heads/master'.
	for _, project := range manifest.GetPinnedProjects() {
		repoPath := r.Harness.GetRemotePath(rh.GetRemoteProject(*project))
		masterSha, err := git.GetGitRepoRevision(repoPath, project.Revision)
		assert.NilError(t, err)
		project.Revision = masterSha
	}
	// Also, to pretend that master is a proper CrOS branch, we need to adjust
	// the multicheckout revisions.
	for _, project := range manifest.GetMultiCheckoutProjects() {
		project.Revision = git.NormalizeRef("master-" + git.StripRefs(project.Revision))
	}

	assert.NilError(t, r.AssertProjectRevisionsMatchBranch(manifest, "master", ""))
	assert.Assert(t, r.AssertProjectRevisionsMatchBranch(manifest, "foo", "") != nil)
}

func TestAssertManifestProjectRepaired(t *testing.T) {
	configManifest := testManifest
	configManifest.Projects = append(configManifest.Projects, DefaultManifestProject)
	config := CrosRepoHarnessConfig{
		Manifest:       configManifest,
		VersionProject: "chromiumos/version",
	}
	r := &CrosRepoHarness{}
	defer r.Teardown()
	err := r.Initialize(&config)
	assert.NilError(t, err)

	// Set up new branch. We have to actually do this because of pinned projects.
	newBranch := "newbranch"
	manifestProject := rh.GetRemoteProject(DefaultManifestProject)
	assert.NilError(t, r.Harness.CreateRemoteRef(manifestProject, newBranch, "master"))

	manifest := r.Harness.Manifest()
	// Deep copy projects so that we can change manifest without changing r.Harness.manifest
	manifest.Projects = append([]repo.Project(nil), manifest.Projects...)

	// Switch the revisions on pinned projects to be SHA-1's instead of 'refs/heads/master'.
	for _, project := range manifest.GetPinnedProjects() {
		pinnedProject := rh.GetRemoteProject(*project)
		repoPath := r.Harness.GetRemotePath(pinnedProject)
		assert.NilError(t, r.Harness.CreateRemoteRef(pinnedProject, newBranch, project.Revision))
		masterSha, err := git.GetGitRepoRevision(repoPath, newBranch)
		assert.NilError(t, err)
		project.Revision = masterSha
	}
	for _, project := range manifest.GetSingleCheckoutProjects() {
		project.Revision = git.NormalizeRef(newBranch)
	}
	for _, project := range manifest.GetMultiCheckoutProjects() {
		project.Revision = git.NormalizeRef(newBranch + "-" + git.StripRefs(project.Revision))
	}

	// Clear default revisions.
	for i := range manifest.Remotes {
		manifest.Remotes[i].Revision = ""
	}
	manifest.Default.Revision = ""

	// Write manifest to file.
	manifestData, err := xml.Marshal(manifest)
	assert.NilError(t, err)
	manifestFile := rh.File{
		Name:     "manifest.xml",
		Contents: []byte(manifestData),
	}
	_, err = r.Harness.AddFile(manifestProject, newBranch, manifestFile)
	assert.NilError(t, err)

	assert.NilError(t, r.AssertManifestProjectRepaired(manifestProject, newBranch, []string{"manifest.xml"}))
}
