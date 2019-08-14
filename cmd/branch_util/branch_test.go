// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"reflect"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
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
	workingManifest = manifest
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[0], ""), "mybranch")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[1], ""), "mybranch-factory-100")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[2], ""), "mybranch-101")
}

func TestProjectBranchName_withOriginal(t *testing.T) {
	manifest := branchNameTestManifest
	workingManifest = manifest
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[3], "oldbranch"), "mybranch-factory-100")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[4], "oldbranch"), "mybranch-factory-101")
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
	workingManifest = manifest
	expected := []ProjectBranch{
		{project: manifest.Projects[0], branchName: "mybranch"},
		{project: manifest.Projects[1], branchName: "mybranch-factory-100"},
	}

	branchNames := projectBranches("mybranch", "oldbranch")
	assert.Assert(t, reflect.DeepEqual(expected, branchNames))
}

func TestGetBranchesByPath(t *testing.T) {
	branches := []ProjectBranch{
		{project: repo.Project{Path: "foo/"}, branchName: "foo-branch"},
		{project: repo.Project{Path: "bar/"}, branchName: "bar-branch"},
	}
	branchMap := map[string]string{
		"foo/": "foo-branch",
		"bar/": "bar-branch",
	}
	assert.Assert(t, reflect.DeepEqual(getBranchesByPath(branches), branchMap))
}

func TestWhichVersionShouldBump_successPatch(t *testing.T) {
	vinfo := repo.VersionInfo{
		ChromeBranch:      0xfa,
		BuildNumber:       0xca,
		BranchBuildNumber: 0xde,
		PatchNumber:       0x00,
	}

	component, err := whichVersionShouldBump(vinfo)
	assert.NilError(t, err)
	assert.Equal(t, component, repo.Patch)
}

func TestWhichVersionShouldBump_successBranch(t *testing.T) {
	vinfo := repo.VersionInfo{
		ChromeBranch:      0xfe,
		BuildNumber:       0xed,
		BranchBuildNumber: 0x00,
		PatchNumber:       0x00,
	}

	component, err := whichVersionShouldBump(vinfo)
	assert.NilError(t, err)
	assert.Equal(t, component, repo.Branch)
}
