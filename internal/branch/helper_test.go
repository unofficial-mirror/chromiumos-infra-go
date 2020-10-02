// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package branch

import (
	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
	"reflect"
	"testing"
)

var branchNameTestManifest = repo.Manifest{
	Projects: []repo.Project{
		// Basic project. Only one checkout, so we can just use the branch name.
		{Path: "bar/", Name: "chromiumos/bar"},
		// Project with multiple checkouts. Upstream/revision will be used as a suffix.
		{Path: "foo1/", Name: "foo", Revision: "100", Upstream: "refs/heads/factory-100"},
		{Path: "foo2/", Name: "foo", Revision: "101"},
		// Project with multiple checkouts that were created as part of a previous branching operation.
		// Will be properly named using the `original` parameter.
		{Path: "baz1/", Name: "baz", Upstream: "refs/heads/oldbranch-factory-100"},
		{Path: "baz2/", Name: "baz", Upstream: "refs/heads/oldbranch-factory-101"},
		{Path: "baz2/", Name: "baz", Upstream: "refs/heads/oldbranch-factory-101"},
		// Project with an upstream that is from a CrOS branch name.
		{Path: "baz2/", Name: "baz", Upstream: "refs/heads/release-R77-12371.B-myfactory/2.6"},

		// Cases covered by the mapping feature
		{Path: "src/third_party/coreboot", Name: "chromiumos/third_party/coreboot", Revision: "8dddd11bc804c01b905b87407e42a2d58d044384", Upstream: "refs/heads/firmware-puff-13324.B-chromeos-2016.05"},
		{Path: "src/third_party/coreboot", Name: "chromiumos/third_party/coreboot", Revision: "8dddd11bc804c01b905b87407e42a2d58d044385"},
	},
}

var canBranchTestManifest = repo.Manifest{
	Projects: []repo.Project{
		{Path: "foo/", Name: "foo",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "create"},
			},
		},
		{Path: "bar/", Name: "bar",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "pin"},
			},
		},
	},
}

func TestProjectBranchName(t *testing.T) {
	manifest := branchNameTestManifest
	WorkingManifest = manifest
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[0], ""), "mybranch")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[1], ""), "mybranch-factory-100")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[2], ""), "mybranch-101")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[6], ""), "mybranch-myfactory-2.6")
}

func TestProjectBranchName_MappingFunctionality(t *testing.T) {
	manifest := branchNameTestManifest
	WorkingManifest = manifest
	assert.Equal(t, projectBranchName("coreboot", manifest.Projects[7], ""), "coreboot")
}

func TestProjectBranchName_withOriginal(t *testing.T) {
	manifest := branchNameTestManifest
	WorkingManifest = manifest
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[3], "oldbranch"), "mybranch-factory-100")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[4], "oldbranch"), "mybranch-factory-101")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[6], "oldbranch"), "mybranch-myfactory-2.6")
}

func TestCanBranchProject(t *testing.T) {
	manifest := canBranchTestManifest
	assert.Assert(t, canBranchProject(manifest, manifest.Projects[0]))
	assert.Assert(t, !canBranchProject(manifest, manifest.Projects[1]))
}

var branchesTestManifest = repo.Manifest{
	Projects: []repo.Project{
		// Basic project. Only one checkout, so we can just use the branch name.
		{Path: "bar/", Name: "chromiumos/bar", Revision: "100", RemoteName: "cros"},
		// Project with multiple checkouts. Upstream/revision will be used as a suffix.
		{Path: "foo1/", Name: "foo", Upstream: "refs/heads/factory-100",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "create"},
			},
		},
		{Path: "foo2/", Name: "foo",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "pin"},
			},
		},
	},
	Remotes: []repo.Remote{
		{Name: "cros"},
	},
}

func TestProjectBranches(t *testing.T) {
	manifest := branchesTestManifest
	WorkingManifest = manifest
	expected := []ProjectBranch{
		{Project: manifest.Projects[0], BranchName: "mybranch"},
		{Project: manifest.Projects[1], BranchName: "mybranch-factory-100"},
	}

	branchNames := ProjectBranches("mybranch", "oldbranch")
	assert.Assert(t, reflect.DeepEqual(expected, branchNames))
}

func TestGetBranchesByPath(t *testing.T) {
	branches := []ProjectBranch{
		{Project: repo.Project{Path: "foo/"}, BranchName: "foo-branch"},
		{Project: repo.Project{Path: "bar/"}, BranchName: "bar-branch"},
	}
	branchMap := map[string]string{
		"foo/": "foo-branch",
		"bar/": "bar-branch",
	}
	assert.Assert(t, reflect.DeepEqual(getBranchesByPath(branches), branchMap))
}

func TestWhichVersionShouldBump_successPatch(t *testing.T) {
	vinfo := mv.VersionInfo{
		ChromeBranch:      0xfa,
		BuildNumber:       0xca,
		BranchBuildNumber: 0xde,
		PatchNumber:       0x00,
	}

	component, err := WhichVersionShouldBump(vinfo)
	assert.NilError(t, err)
	assert.Equal(t, component, mv.Patch)
}

func TestWhichVersionShouldBump_successBranch(t *testing.T) {
	vinfo := mv.VersionInfo{
		ChromeBranch:      0xfe,
		BuildNumber:       0xed,
		BranchBuildNumber: 0x00,
		PatchNumber:       0x00,
	}

	component, err := WhichVersionShouldBump(vinfo)
	assert.NilError(t, err)
	assert.Equal(t, component, mv.Branch)
}

func TestNewBranchName_Custom(t *testing.T) {
	assert.Equal(t, NewBranchName(mv.VersionInfo{}, "custom-name", "", false, false, false, false), "custom-name")
}

var vinfo = mv.VersionInfo{
	ChromeBranch:      77,
	BuildNumber:       123,
	BranchBuildNumber: 1,
	PatchNumber:       0,
}

func TestNewBranchName_Release(t *testing.T) {
	assert.Equal(t, NewBranchName(vinfo, "", "", true, false, false, false), "release-R77-123.1.B")
}

func TestNewBranchName_Factory(t *testing.T) {
	assert.Equal(t, NewBranchName(vinfo, "", "foo", false, true, false, false), "factory-foo-123.1.B")
}

func TestNewBranchName_Firmware(t *testing.T) {
	assert.Equal(t, NewBranchName(vinfo, "", "", false, false, true, false), "firmware-123.1.B")
}

func TestNewBranchName_Stabilize(t *testing.T) {
	assert.Equal(t, NewBranchName(vinfo, "", "", false, false, false, true), "stabilize-123.1.B")
}
