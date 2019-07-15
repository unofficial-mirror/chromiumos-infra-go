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

func TestNewBranchName_Custom(t *testing.T) {
	c := &createBranchRun{
		custom: "custom-name",
	}
	assert.Equal(t, c.newBranchName(), "custom-name")
}

var vinfo = repo.VersionInfo{
	ChromeBranch:      77,
	BuildNumber:       123,
	BranchBuildNumber: 1,
	PatchNumber:       0,
}

func TestNewBranchName_Release(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	c := &createBranchRun{
		release: true,
	}
	m.EXPECT().
		ReadVersion().
		Return(vinfo, nil)
	assert.Equal(t, c.newBranchName(), "release-R77-123.1.B")
}

func TestNewBranchName_Factory(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	c := &createBranchRun{
		factory:    true,
		descriptor: "foo",
	}
	m.EXPECT().
		ReadVersion().
		Return(vinfo, nil)
	assert.Equal(t, c.newBranchName(), "factory-foo-123.1.B")
}

func TestNewBranchName_Firmware(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m
	c := &createBranchRun{
		firmware: true,
	}
	m.EXPECT().
		ReadVersion().
		Return(vinfo, nil)
	assert.Equal(t, c.newBranchName(), "firmware-123.1.B")
}

func TestNewBranchName_Stabilize(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	checkout = m

	c := &createBranchRun{
		stabilize: true,
	}
	m.EXPECT().
		ReadVersion().
		Return(vinfo, nil)
	assert.Equal(t, c.newBranchName(), "stabilize-123.1.B")
}
