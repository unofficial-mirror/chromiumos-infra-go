// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo

import (
	"gotest.tools/assert"
	"testing"
)

func assertVersionEqual(t *testing.T, v VersionInfo, expected []int) {
	assert.Equal(t, v.ChromeBranch, expected[0])
	assert.Equal(t, v.BuildNumber, expected[1])
	assert.Equal(t, v.BranchBuildNumber, expected[2])
	assert.Equal(t, v.PatchNumber, expected[3])
}

func TestGetVersionInfoFromRepo_success(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 1, 0})
}

func TestIncrementVersion_ChromeBranch(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(ChromeBranch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{78, 12303, 0, 0})
}

func TestIncrementVersion_Build(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(Build)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12303, 0, 0})
}

func TestIncrementVersion_Branch(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(Branch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 2, 0})
}

func TestIncrementVersion_Branch_nonzero(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.PatchNumber = 1
	versionInfo.IncrementVersion(Branch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 1, 2})
}

func TestIncrementVersion_Patch(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(Patch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 1, 1})
}

func TestVersionString(t *testing.T) {
	var v VersionInfo
	v.BuildNumber = 123
	v.BranchBuildNumber = 1
	v.PatchNumber = 0
	assert.Equal(t, v.VersionString(), "123.1.0")
}

func TestVersionComponents(t *testing.T) {
	var v VersionInfo
	v.BuildNumber = 123
	v.BranchBuildNumber = 1
	v.PatchNumber = 0
	components := []int{123, 1, 0}
	assert.DeepEqual(t, v.VersionComponents(), components)
}

func TestStrippedVersionString(t *testing.T) {
	var v VersionInfo
	v.BuildNumber = 123
	assert.Equal(t, v.StrippedVersionString(), "123")
	v.BranchBuildNumber = 1
	assert.Equal(t, v.StrippedVersionString(), "123.1")
}
