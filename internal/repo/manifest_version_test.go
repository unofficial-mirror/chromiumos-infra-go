// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"gotest.tools/assert"
)

func TestVersionsEqual(t *testing.T) {
	a := VersionInfo{
		ChromeBranch:      1,
		BuildNumber:       2,
		BranchBuildNumber: 3,
		PatchNumber:       4,
	}
	b := a
	b.BranchBuildNumber = 5
	assert.Assert(t, VersionsEqual(a, a))
	assert.Assert(t, !VersionsEqual(a, b))
}

func assertVersionEqual(t *testing.T, v VersionInfo, expected []int) {
	assert.Equal(t, v.ChromeBranch, expected[0])
	assert.Equal(t, v.BuildNumber, expected[1])
	assert.Equal(t, v.BranchBuildNumber, expected[2])
	assert.Equal(t, v.PatchNumber, expected[3])
}

func TestGetVersionInfoFromRepo_success(t *testing.T) {
	VersionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 1, 0})
}

const versionFileContents string = `
if [ -z "${FLAGS_version}" ]; then
  # Release Build number.
  # Increment by 1 for every release build.
  CHROMEOS_BUILD=12302

  # Release Branch number.
  # Increment by 1 for every release build on a branch.
  # Reset to 0 when increasing release build number.
  CHROMEOS_BRANCH=1

  # Patch number.
  # Increment by 1 in case a non-scheduled branch release build is necessary.
  # Reset to 0 when increasing branch number.
  CHROMEOS_PATCH=0

  # Official builds must set CHROMEOS_OFFICIAL=1.
  if [ ${CHROMEOS_OFFICIAL:-0} -ne 1 ]; then
    # For developer builds, overwrite CHROMEOS_PATCH with a date string
    # for use by auto-updater.
    CHROMEOS_PATCH=$(date +%Y_%m_%d_%H%M)
  fi

  # Version string. Not indentied to appease bash.
  CHROMEOS_VERSION_STRING=\
"${CHROMEOS_BUILD}.${CHROMEOS_BRANCH}.${CHROMEOS_PATCH}"
else
  CHROMEOS_BUILD=$(echo "${FLAGS_version}" | cut -f 1 -d ".")
  CHROMEOS_BRANCH=$(echo "${FLAGS_version}" | cut -f 2 -d ".")
  CHROMEOS_PATCH=$(echo "${FLAGS_version}" | cut -f 3 -d ".")
  CHROMEOS_VERSION_STRING="${FLAGS_version}"
fi

# Major version for Chrome.
CHROME_BRANCH=77
`

func TestParseVersionInfo_success(t *testing.T) {
	versionInfo, err := ParseVersionInfo([]byte(versionFileContents))
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 1, 0})
}

func TestParseVersionInfo_error(t *testing.T) {
	_, err := ParseVersionInfo([]byte("foo"))
	assert.ErrorContains(t, err, "did not find field")
}

func TestIncrementVersion_ChromeBranch(t *testing.T) {
	VersionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(ChromeBranch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{78, 12303, 0, 0})
}

func TestIncrementVersion_Build(t *testing.T) {
	VersionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(Build)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12303, 0, 0})
}

func TestIncrementVersion_Branch(t *testing.T) {
	VersionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.IncrementVersion(Branch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 2, 0})
}

func TestIncrementVersion_Branch_nonzero(t *testing.T) {
	VersionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data")
	versionInfo.PatchNumber = 1
	versionInfo.IncrementVersion(Branch)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []int{77, 12302, 1, 2})
}

func TestIncrementVersion_Patch(t *testing.T) {
	VersionFilePath = "chromeos_version.sh"
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

func TestUpdateVersionFile_noVersionFile(t *testing.T) {
	var v VersionInfo
	err := v.UpdateVersionFile("", false, git.RemoteRef{})
	assert.ErrorContains(t, err, "associated version file")
}

func TestUpdateVersionFile_success(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)
	tmpPath := filepath.Join(tmpDir, "chromeos_version.sh")

	// We're modifying chromeos_version.sh, so need to copy it to  a tmp file.
	fileContents, err := ioutil.ReadFile("test_data/chromeos_version.sh")
	assert.NilError(t, err)
	err = ioutil.WriteFile(tmpPath, fileContents, 0644)
	assert.NilError(t, err)

	commitMsg := "commit"
	remoteRef := git.RemoteRef{
		Remote: "remote",
		Ref:    "ref",
	}

	// Set git mock expectations.
	pushRefs := fmt.Sprintf("%s:%s", pushBranch, remoteRef.Ref)
	git.CommandRunnerImpl = &cmd.FakeCommandRunnerMulti{
		CommandRunners: []cmd.FakeCommandRunner{
			{
				ExpectedDir: tmpDir,
				ExpectedCmd: []string{"git", "checkout", "-B", pushBranch},
			},
			{
				ExpectedDir: tmpDir,
				ExpectedCmd: []string{"git", "add", "-A"},
			},
			{
				ExpectedDir: tmpDir,
				ExpectedCmd: []string{"git", "commit", "-m", commitMsg},
			},
			{
				ExpectedDir: tmpDir,
				ExpectedCmd: []string{"git", "rev-parse", "HEAD"},
			},
			{
				ExpectedDir: tmpDir,
				ExpectedCmd: []string{"git", "push", remoteRef.Remote, pushRefs, "--dry-run"},
			},
		},
	}

	// Call UpdateVersionFile.
	var v VersionInfo
	v.ChromeBranch = 1337
	v.BuildNumber = 0xdead
	v.BranchBuildNumber = 0xbeef
	v.PatchNumber = 0
	v.VersionFile = tmpPath
	err = v.UpdateVersionFile(commitMsg, true, remoteRef)
	assert.NilError(t, err)

	// Read version info back in from file, make sure it's correct.
	versionInfo, err := GetVersionInfoFromRepo(tmpDir)
	assert.NilError(t, err)
	assert.Equal(t, versionInfo, v)
}
