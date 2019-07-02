package repo

import (
	"gotest.tools/assert"
	"testing"
)

func assertVersionEqual(t *testing.T, v VersionInfo, expected []string) {
	assert.Equal(t, v.ChromeBranch, expected[0])
	assert.Equal(t, v.BuildNumber, expected[1])
	assert.Equal(t, v.BranchBuildNumber, expected[2])
	assert.Equal(t, v.PatchNumber, expected[3])
}

func TestGetVersionInfoFromRepo_success(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data", Build)
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []string{"77", "12302", "1", "0"})
}

func TestIncrementVersion_ChromeBranch(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data", ChromeBranch)
	versionInfo.IncrementVersion()
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []string{"78", "12303", "0", "0"})
}

func TestIncrementVersion_Build(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data", Build)
	versionInfo.IncrementVersion()
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []string{"77", "12303", "0", "0"})
}

func TestIncrementVersion_Branch(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data", Branch)
	versionInfo.IncrementVersion()
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []string{"77", "12302", "2", "0"})
}

func TestIncrementVersion_Branch_nonzero(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data", Branch)
	versionInfo.PatchNumber = "1"
	versionInfo.IncrementVersion()
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []string{"77", "12302", "1", "2"})
}

func TestIncrementVersion_Patch(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	versionInfo, err := GetVersionInfoFromRepo("test_data", Patch)
	versionInfo.IncrementVersion()
	assert.NilError(t, err)
	assertVersionEqual(t, versionInfo, []string{"77", "12302", "1", "1"})
}

func TestVersionString(t *testing.T) {
	versionFilePath = "chromeos_version.sh"
	var v VersionInfo
	v.BuildNumber = "123"
	v.BranchBuildNumber = "1"
	v.PatchNumber = "0"
	assert.Equal(t, v.VersionString(), "123.1.0")
}
