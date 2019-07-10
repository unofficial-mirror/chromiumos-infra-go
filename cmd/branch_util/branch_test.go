// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"github.com/golang/mock/gomock"
	mock_checkout "go.chromium.org/chromiumos/infra/go/internal/checkout/mock"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
	"testing"
)

var testManifest = repo.Manifest{
	Projects: []repo.Project{
		{Path: "foo1/", Name: "foo", Revision: "100", Upstream: "refs/heads/factory-100"},
		{Path: "foo2/", Name: "foo", Revision: "101"},
		{Path: "bar/", Name: "bar"},
		{Path: "baz1/", Name: "baz", Upstream: "refs/heads/oldbranch-factory-100"},
		{Path: "baz2/", Name: "baz", Upstream: "refs/heads/oldbranch-factory-101"},
	},
}

func TestProjectBranchName(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	c := &createBranchRun{}
	m.EXPECT().
		Manifest().
		Return(testManifest).
		AnyTimes()
	assert.Equal(t, c.projectBranchName("mybranch", testManifest.Projects[0], ""), "mybranch-factory-100")
	assert.Equal(t, c.projectBranchName("mybranch", testManifest.Projects[1], ""), "mybranch-101")
	assert.Equal(t, c.projectBranchName("mybranch", testManifest.Projects[2], ""), "mybranch")
}

func TestProjectBranchName_withOriginal(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	c := &createBranchRun{}
	m.EXPECT().
		Manifest().
		Return(testManifest).
		AnyTimes()
	assert.Equal(t, c.projectBranchName("mybranch", testManifest.Projects[3], "oldbranch"), "mybranch-factory-100")
	assert.Equal(t, c.projectBranchName("mybranch", testManifest.Projects[4], "oldbranch"), "mybranch-factory-101")
}
