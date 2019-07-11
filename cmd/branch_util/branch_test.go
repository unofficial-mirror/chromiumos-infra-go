// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/golang/mock/gomock"
	mock_checkout "go.chromium.org/chromiumos/infra/go/internal/checkout/mock"
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
		// Projects with annotations labeling branch mode.
		{Path: "foo1/", Name: "foo",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "create"},
			},
		},
		{Path: "foo2/", Name: "foo",
			Annotations: []repo.Annotation{
				{Name: "branch-mode", Value: "pin"},
			},
		},
		// Remote has name but no alias. Project is branchable.
		{Path: "bar/", Name: "chromiumos/bar", RemoteName: "cros"},
		// Remote has alias. Project is branchable.
		{Path: "baz1/", Name: "aosp/baz", RemoteName: "cros1"},
		// Remote has alias. Remote is not a cros remote.
		{Path: "baz2/", Name: "aosp/baz", RemoteName: "cros2"},
		// Remote has alias. Remote is a cros remote, but not a branchable one.
		{Path: "fizz/", Name: "fizz", RemoteName: "cros"},
		// Remote has name but no alias. Remote is a branchable remote, but specific
		// project is not branchable.
		{Path: "buzz/", Name: "buzz", RemoteName: "weave"},
	},
	Remotes: []repo.Remote{
		{Name: "cros"},
		{Name: "cros1", Alias: "cros"},
		{Name: "cros2", Alias: "github"},
		{Name: "weave"},
	},
}

func TestProjectBranchName(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	manifest := branchNameTestManifest
	m.EXPECT().
		Manifest().
		Return(manifest).
		AnyTimes()
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[0], ""), "mybranch")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[1], ""), "mybranch-factory-100")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[2], ""), "mybranch-101")
}

func TestProjectBranchName_withOriginal(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	manifest := branchNameTestManifest
	m.EXPECT().
		Manifest().
		Return(manifest).
		AnyTimes()
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[3], "oldbranch"), "mybranch-factory-100")
	assert.Equal(t, projectBranchName("mybranch", manifest.Projects[4], "oldbranch"), "mybranch-factory-101")
}

func TestCanBranchProject_annotation(t *testing.T) {
	manifest := canBranchTestManifest
	assert.Assert(t, canBranchProject(manifest, manifest.Projects[0]))
	assert.Assert(t, !canBranchProject(manifest, manifest.Projects[1]))
}

func TestCanBranchProject_remote(t *testing.T) {
	manifest := canBranchTestManifest
	// Remote has name but no alias. Project is branchable.
	assert.Assert(t, canBranchProject(manifest, manifest.Projects[2]))
	// Remote has alias. Project is branchable.
	assert.Assert(t, canBranchProject(manifest, manifest.Projects[3]))
	// Remote has alias. Remote is not a cros remote.
	assert.Assert(t, !canBranchProject(manifest, manifest.Projects[4]))
	// Remote has alias. Remote is a cros remote, but not a branchable one.
	assert.Assert(t, !canBranchProject(manifest, manifest.Projects[6]))
	// Remote has name but no alias. Remote is a branchable remote, but specific
	// project is not branchable.
	assert.Assert(t, !canBranchProject(manifest, manifest.Projects[5]))
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
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	manifest := branchesTestManifest
	m.EXPECT().
		Manifest().
		Return(manifest).
		AnyTimes()

	expected := []ProjectBranch{
		{project: manifest.Projects[0], branchName: "mybranch"},
		{project: manifest.Projects[1], branchName: "mybranch-factory-100"},
	}

	branchNames := projectBranches("mybranch", "oldbranch")
	assert.Assert(t, reflect.DeepEqual(expected, branchNames))
}

func TestAssertBranchesDoNotExist(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m

	projectBranches := []ProjectBranch{
		{
			project: repo.Project{
				Name: "myProject",
			},
			branchName: "myBranch",
		},
	}
	branchNameRegexp := regexp.MustCompile(projectBranches[0].branchName)

	m.EXPECT().
		BranchExists(
			gomock.Eq(projectBranches[0].project),
			gomock.Eq(branchNameRegexp)).
		Return(false, nil)

	err := assertBranchesDoNotExist(projectBranches)
	assert.NilError(t, err)

	m.EXPECT().
		BranchExists(
			gomock.Eq(projectBranches[0].project),
			gomock.Eq(branchNameRegexp)).
		Return(false, fmt.Errorf("branch exist error"))

	err = assertBranchesDoNotExist(projectBranches)
	assert.ErrorContains(t, err, "branch exist")

	m.EXPECT().
		BranchExists(
			gomock.Eq(projectBranches[0].project),
			gomock.Eq(branchNameRegexp)).
		Return(true, nil)

	err = assertBranchesDoNotExist(projectBranches)
	assert.ErrorContains(t, err, "rerun with --force")
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
