// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/cmd/branch_util/test"
	"go.chromium.org/chromiumos/infra/go/internal/branch"
	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	gerrit "go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	rh "go.chromium.org/chromiumos/infra/go/internal/repo_harness"
	"go.chromium.org/chromiumos/infra/go/internal/test_util"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"gotest.tools/assert"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	defaultFileName  = "default.xml"
	officialFileName = "official.xml"
	externalFileName = "external.xml"
	internalFileName = "internal.xml"
	remotesFileName  = "_remotes.xml"

	defaultXML = `
  <default revision="refs/heads/main" remote="cros" sync-j="8"/>
`
	remoteExternalXML = `
  <remote name="cros" revision="refs/heads/main" fetch="%s"/>
`
	remoteInternalXML = `
  <remote name="cros-internal" revision="refs/heads/main" fetch="%s"/>
`
	projectsExternalXML = `
  <project path="src/repohooks" name="chromiumos/repohooks"
           groups="minilayout,firmware,buildtools,labtools,crosvm" />
  <repo-hooks in-project="chromiumos/repohooks" enabled-list="pre-upload" />

  <!--This comment should persist.-->
  <project name="chromiumos/manifest" path="manifest"/>

  <new-element name="this should persist" />

  <project name="chromiumos/overlays/chromiumos-overlay"
           path="src/third_party/chromiumos-overlay"/>

  <project name="external/implicit-pinned"
           path="src/third_party/implicit-pinned"
           revision="refs/heads/implicit-pinned"/>

  <!--This comment should also persist.-->
  <project name="chromiumos/multicheckout"
           path="src/third_party/multicheckout-a"
           revision="refs/heads/multicheckout-a"/>

  <project name="chromiumos/multicheckout"
           path="src/third_party/multicheckout-b"
           revision="refs/heads/multicheckout-b"/>
`
	projectsInternalXML = `
  <project name="chromeos/manifest-internal"
           path="manifest-internal"
           remote="cros-internal"
           upstream="refs/heads/main"/>

  <project name="chromeos/explicit-pinned"
           path="src/explicit-pinned"
           revision="refs/heads/explicit-pinned"
           remote="cros-internal">
    <annotation name="branch-mode" value="pin"/>
  </project>

  <project name="chromeos/explicit-branch"
           path="src/explicit-branch"
           remote="cros-internal">
    <annotation name="branch-mode" value="create"/>
  </project>

  <project name="chromeos/explicit-tot"
           path="src/explicit-tot"
           remote="cros-internal">
    <annotation name="branch-mode" value="tot"/>
  </project>
`
	includeRemotesXML = `
  <include name="_remotes.xml"/>
`
	includeExternalXML = `
  <include name="external.xml"/>
`
	includeInternalXML = `
  <include name="internal.xml"/>
`
)

const (
	existingBranchName = "old-branch"

	defaultBranchedXML = `
  <default remote="cros" sync-j="8"/>
`
	remoteExternalBranchedXML = `
  <remote name="cros" fetch="%s"/>
`
	remoteInternalBranchedXML = `
  <remote name="cros-internal" fetch="%s"/>
`

	projectsExternalBranchedXML = `
  <project path="src/repohooks" name="chromiumos/repohooks"
           groups="minilayout,firmware,buildtools,labtools,crosvm"
           revision="refs/heads/%[1]s" />
  <repo-hooks in-project="chromiumos/repohooks" enabled-list="pre-upload" />

  <!--This comment should persist.-->
  <project name="chromiumos/manifest"
           path="manifest"
           revision="refs/heads/%[1]s"/>

  <new-element name="this should persist" />

  <project name="chromiumos/overlays/chromiumos-overlay"
           path="src/third_party/chromiumos-overlay"
           revision="refs/heads/%[1]s"/>

  <project name="external/implicit-pinned"
           path="src/third_party/implicit-pinned"
           revision="refs/heads/implicit-pinned"/>

  <!--This comment should also persist.-->
  <project name="chromiumos/multicheckout"
           path="src/third_party/multicheckout-a"
           revision="refs/heads/%[1]s-multicheckout-a"/>

  <project name="chromiumos/multicheckout"
           path="src/third_party/multicheckout-b"
           revision="refs/heads/%[1]s-multicheckout-b"/>
`

	projectsInternalBranchedXML = `
  <project name="chromeos/manifest-internal"
           path="manifest-internal"
           remote="cros-internal"
           revision="refs/heads/%[1]s"
           upstream="refs/heads/%[1]s"/>

  <project name="chromeos/explicit-pinned"
           path="src/explicit-pinned"
           revision="refs/heads/explicit-pinned"
           remote="cros-internal">
    <annotation name="branch-mode" value="pin"/>
  </project>

  <project name="chromeos/explicit-branch"
           path="src/explicit-branch"
           remote="cros-internal"
           revision="refs/heads/%[1]s">
    <annotation name="branch-mode" value="create"/>
  </project>

  <project name="chromeos/explicit-tot"
           path="src/explicit-tot"
           remote="cros-internal"
           revision="refs/heads/main">
    <annotation name="branch-mode" value="tot"/>
  </project>
`
)

const chromeVersionMock = `
	CHROMEOS_BUILD=13324
	CHROMEOS_BRANCH=10
	CHROMEOS_PATCH=0
	CHROME_BRANCH=86
`

var (
	manifestProject = rh.RemoteProject{
		RemoteName:  "cros",
		ProjectName: "chromiumos/manifest",
	}
	manifestInternalProject = rh.RemoteProject{
		RemoteName:  "cros-internal",
		ProjectName: "chromeos/manifest-internal",
	}
	application = getApplication(chromeinfra.DefaultAuthOptions())

	expectedBranchVersion = mv.VersionInfo{
		ChromeBranch:      86,
		BuildNumber:       13324,
		BranchBuildNumber: 10,
		PatchNumber:       0,
	}
)

func getManifestFiles(crosFetch, crosInternalFetch string) (
	manifestFiles, manifestInternalFiles map[string]string, fullTotXML string) {
	// We use a dummy value here because if a <	remote> tag has fetch="",
	// it will default to the actual gerrit remote.
	if crosFetch == "" {
		crosFetch = "placeholder"
	}
	if crosInternalFetch == "" {
		crosInternalFetch = "placeholder"
	}
	remoteInternalXML := fmt.Sprintf(remoteInternalXML, crosInternalFetch)
	remoteExternalXML := fmt.Sprintf(remoteExternalXML, crosFetch)

	manifestFiles = map[string]string{
		remotesFileName: manifestXml(remoteExternalXML),
		externalFileName: manifestXml(
			defaultXML, includeRemotesXML, projectsExternalXML),
		defaultFileName: manifestXml(includeExternalXML),
	}

	manifestInternalFiles = map[string]string{
		remotesFileName:  manifestXml(remoteExternalXML, remoteInternalXML),
		externalFileName: manifestFiles[externalFileName],
		internalFileName: manifestXml(
			defaultXML, includeRemotesXML, projectsInternalXML),
		officialFileName: manifestXml(
			includeInternalXML, includeExternalXML),
		defaultFileName: manifestXml(
			includeInternalXML, includeExternalXML),
	}

	fullTotXML = manifestXml(
		defaultXML,
		remoteExternalXML,
		remoteInternalXML,
		projectsExternalXML,
		projectsInternalXML,
	)
	return
}

func getBranchedManifestFiles(branch, crosFetch, crosInternalFetch string) (
	manifestBranchedFiles map[string]string,
	manifestInternalBranchedFiles map[string]string,
	fullBranchedXML string) {
	// We use a dummy value here because if a <	remote> tag has fetch="",
	// it will default to the actual gerrit remot
	if crosFetch == "" {
		crosFetch = "placeholder"
	}
	if crosInternalFetch == "" {
		crosInternalFetch = "placeholder"
	}
	remoteInternalXML := fmt.Sprintf(remoteInternalBranchedXML, crosInternalFetch)
	remoteExternalXML := fmt.Sprintf(remoteExternalBranchedXML, crosFetch)
	projectsExternalBranchedXML := fmt.Sprintf(projectsExternalBranchedXML, branch)
	projectsInternalBranchedXML := fmt.Sprintf(projectsInternalBranchedXML, branch)

	manifestBranchedFiles = map[string]string{
		remotesFileName: manifestXml(remoteExternalXML),
		externalFileName: manifestXml(
			defaultBranchedXML, includeRemotesXML, projectsExternalBranchedXML),
		defaultFileName: manifestXml(includeExternalXML),
	}

	manifestInternalBranchedFiles = map[string]string{
		remotesFileName:  manifestXml(remoteExternalXML, remoteInternalXML),
		externalFileName: manifestBranchedFiles[externalFileName],
		internalFileName: manifestXml(
			defaultXML, includeRemotesXML, projectsInternalBranchedXML),
		officialFileName: manifestXml(
			includeInternalXML, includeExternalXML),
		defaultFileName: manifestXml(
			includeInternalXML, includeExternalXML),
	}

	fullBranchedXML = manifestXml(
		defaultBranchedXML,
		remoteExternalXML,
		remoteInternalXML,
		projectsExternalBranchedXML,
		projectsInternalBranchedXML,
	)
	return
}

func getExistingBranchManifestFiles(crosFetch, crosInternalFetch string) (
	manifestBranchedFiles map[string]string,
	manifestInternalBranchedFiles map[string]string,
	fullBranchedXML string) {
	return getBranchedManifestFiles(existingBranchName, crosFetch, crosInternalFetch)
}

func manifestXml(chunks ...string) string {
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>
		<manifest>
		%s
		</manifest>`,
		strings.Join(chunks, ""))
}

func getDefaultConfig() test.CrosRepoHarnessConfig {
	_, _, fullTotXML := getManifestFiles("", "")

	var manifest repo.Manifest
	err := xml.Unmarshal([]byte(fullTotXML), &manifest)
	if err != nil {
		log.Fatalf("failed to parse manifest: %s", err.Error())
	}
	config := test.CrosRepoHarnessConfig{
		Manifest:       manifest,
		VersionProject: "chromiumos/overlays/chromiumos-overlay",
	}
	return config
}

func fullManifestPath(r *test.CrosRepoHarness) string {
	return filepath.Join(r.Harness.HarnessRoot(), "manifest.xml")
}

func fullBranchedManifestPath(r *test.CrosRepoHarness) string {
	return filepath.Join(r.Harness.HarnessRoot(), "manifest-branched.xml")
}

func addManifestFiles(t *testing.T,
	r *test.CrosRepoHarness,
	project rh.RemoteProject,
	branch string,
	files map[string]string) {

	filesToAdd := []rh.File{}
	for file, contents := range files {
		filesToAdd = append(filesToAdd, rh.File{
			Name:     file,
			Contents: []byte(contents),
		})
	}
	_, err := r.Harness.AddFiles(project, branch, filesToAdd)
	assert.NilError(t, err)
}

func setUp(t *testing.T) *test.CrosRepoHarness {
	config := getDefaultConfig()
	var r test.CrosRepoHarness
	assert.NilError(t, r.Initialize(&config))

	// Write version.
	version := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.SetVersion("", version))

	// Write full tot manifest to a file so that it can be passed to cros branch.
	// CrosRepoHarness modifies the manifest it is passed (e.g. updates fetch attributes),
	// so use the updated version.
	manifest := r.Harness.Manifest()
	manifestData, err := xml.Marshal(manifest)
	assert.NilError(t, err)
	assert.NilError(t, ioutil.WriteFile(fullManifestPath(&r), manifestData, 0777))

	// Because we use a hierachy of manifest files, we need to update the fetch attributes
	// in _remotes.xml.
	crosFetchVal := manifest.GetRemoteByName("cros").Fetch
	crosInternalFetchVal := manifest.GetRemoteByName("cros-internal").Fetch
	manifestFiles, manifestInternalFiles, _ := getManifestFiles(crosFetchVal, crosInternalFetchVal)

	manifestBranchedFiles, manifestInternalBranchedFiles, fullBranchedXML :=
		getExistingBranchManifestFiles(crosFetchVal, crosInternalFetchVal)

	// Add manifest files to remote.
	addManifestFiles(t, &r, manifestProject, "main", manifestFiles)

	// Add manifest-internal files to remote.
	addManifestFiles(t, &r, manifestInternalProject, "main", manifestInternalFiles)

	// Create existing branch on remote.
	var branchManifest *repo.Manifest
	assert.NilError(t, xml.Unmarshal([]byte(fullBranchedXML), &branchManifest))
	branchManifest = branchManifest.ResolveImplicitLinks()
	// Write full branched manifest to file so that it can be passed to cros branch in
	// *Nonmain tests.
	assert.NilError(t, ioutil.WriteFile(fullBranchedManifestPath(&r), []byte(fullBranchedXML), 0777))

	// Create Ref for each project.
	for _, project := range branchManifest.Projects {
		projectBranch := git.StripRefs(project.Revision)
		err = r.Harness.CreateRemoteRef(rh.GetRemoteProject(project), projectBranch, "")
		if err != nil && strings.Contains(err.Error(), "already exists") {
			continue
		}
		assert.NilError(t, err)
	}
	// Set version file.
	version = mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       2,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.SetVersion(existingBranchName, version))
	// Commit manifest files.
	addManifestFiles(t, &r, manifestProject, existingBranchName, manifestBranchedFiles)
	addManifestFiles(t, &r, manifestInternalProject, existingBranchName, manifestInternalBranchedFiles)

	assert.NilError(t, r.TakeSnapshot())

	return &r
}

// Get the keys of a map[string]string.
func getKeys(m map[string]string) []string {
	keys := make([]string, len(m))
	idx := 0
	for k := range m {
		keys[idx] = k
		idx += 1
	}
	return keys
}

func assertManifestsRepaired(t *testing.T, r *test.CrosRepoHarness, branch string) {
	manifestFiles, manifestInternalFiles, _ := getManifestFiles("", "")
	assert.NilError(t, r.AssertManifestProjectRepaired(
		manifestProject, branch, getKeys(manifestFiles)))
	assert.NilError(t, r.AssertManifestProjectRepaired(
		manifestInternalProject, branch, getKeys(manifestInternalFiles)))
}

func assertCommentsPersist(t *testing.T, r *test.CrosRepoHarness,
	sourceFiles func(string, string) (map[string]string, map[string]string, string), branch string) {
	manifestFiles, manifestInternalFiles, _ := sourceFiles("", "")
	assert.NilError(t, r.AssertCommentsPersist(manifestProject, branch, manifestFiles))
	assert.NilError(t, r.AssertCommentsPersist(manifestInternalProject, branch, manifestInternalFiles))
}

func assertMinimalManifestChanges(t *testing.T, r *test.CrosRepoHarness, branch string) {
	// Ensure that the created manifests differ minimally from the expected manifests (as produced by
	// getBranchedManifestFiles).
	expectedManifestFiles, expectedManifestInternalFiles, _ := getBranchedManifestFiles(branch, "", "")
	assert.NilError(t, r.AssertMinimalManifestChanges(manifestProject, branch, expectedManifestFiles))
	assert.NilError(t, r.AssertMinimalManifestChanges(manifestInternalProject, branch, expectedManifestInternalFiles))
}

func assertNoRemoteDiff(t *testing.T, r *test.CrosRepoHarness) {
	manifest := r.Harness.Manifest()
	for _, remote := range manifest.Remotes {
		remotePath := filepath.Join(r.Harness.HarnessRoot(), remote.Name)
		remoteSnapshot, err := r.GetRecentRemoteSnapshot(remote.Name)
		assert.NilError(t, err)
		assert.NilError(t, test_util.AssertContentsEqual(remoteSnapshot, remotePath))
	}
}

// createSetUp creates the neccessary mocks we need to test the create-v2 function
func createSetUp(t *testing.T) (gitilespb.GitilesClient, error) {
	r := setUp(t)
	defer r.Teardown()

	// Get manifest contents for return
	manifestPath := fullManifestPath(r)
	manifestFile, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	manifest := string(manifestFile)

	// Mock Gitiles controller
	ctl := gomock.NewController(t)

	// Mock manifest request
	reqManifest := &gitilespb.DownloadFileRequest{
		Project:    "manifest",
		Path:       "01/1234.2.0.xml",
		Committish: "main",
		Format:     gitilespb.DownloadFileRequest_TEXT,
	}

	// Mock version file request
	reqVersionFile := &gitilespb.DownloadFileRequest{
		Project:    "version",
		Path:       "chromeos_version.sh",
		Committish: "main",
		Format:     gitilespb.DownloadFileRequest_TEXT,
	}

	// Mock download response
	gitilesMock := gitilespb.NewMockGitilesClient(ctl)
	gitilesMock.EXPECT().DownloadFile(gomock.Any(), reqManifest).Return(
		&gitilespb.DownloadFileResponse{
			Contents: manifest,
		},
		nil,
	)
	gitilesMock.EXPECT().DownloadFile(gomock.Any(), reqVersionFile).Return(
		&gitilespb.DownloadFileResponse{
			Contents: chromeVersionMock,
		},
		nil,
	)

	return gitilesMock, nil
}

// branchCreationTester recreates the branching process as seen in create@
func branchCreationTester(manifestInternal repo.Project, vinfo mv.VersionInfo,
	expectedBranchName, customBranchName, descriptor string, release, factory, firmware, stabilize bool) error {

	sourceRevision := manifestInternal.Revision
	sourceUpstream := git.StripRefs(manifestInternal.Upstream)

	branchType := ""
	switch {
	case release:
		branchType = "release"
	case factory:
		branchType = "factory"
	case firmware:
		branchType = "firmware"
	case stabilize:
		branchType = "stabilize"
	default:
		branchType = "custom"

	}

	// Check if branched
	if err := branch.CheckIfAlreadyBranched(vinfo, manifestInternal, false, branchType); err != nil {
		return fmt.Errorf("Error: %s", err.Error())
	}

	branchName := branch.NewBranchName(vinfo, customBranchName, descriptor, release, factory, firmware, stabilize)
	if branchName != expectedBranchName {
		return fmt.Errorf("%s does not match expected branch name %s", branchName, expectedBranchName)
	}

	componentToBump, err := branch.WhichVersionShouldBump(vinfo)

	if componentToBump != mv.Patch {
		return fmt.Errorf("incorrect VersionComponent selected to be bumped")
	}

	// Get project branches
	branches := branch.ProjectBranches(branchName, git.StripRefs(sourceRevision))

	// Check branch name creation
	for _, branchProject := range branches {
		if !strings.Contains(branchProject.BranchName, branchName) {
			return fmt.Errorf("incorrect branch name created")
		}

	}

	// Gerrit remote branch creation
	projectBranches, err := branch.GerritProjectBranches(branches)

	for _, project := range projectBranches {
		if !strings.Contains(project.Branch, branchName) {
			return fmt.Errorf("incorrect branch name created")
		}
	}

	if projectBranches == nil || err != nil {
		return fmt.Errorf("gerrit branch creation error")

	}

	// Bump version number
	if err = branch.BumpForCreate(componentToBump, release, false, branchName, sourceUpstream); err != nil {
		return fmt.Errorf("Failed to bump version reason: %s", err.Error())
	}

	return nil
}

// TestCreateV2 performs unit tests on the components of create-v2
func TestCreateV2(t *testing.T) {
	// Get Mock for gitiles download
	mockGitiles, err := createSetUp(t)
	if err != nil {
		t.Error("Error: CreaveV2 setup failed. reason: " + err.Error())
		return
	}

	ctx := context.Background()
	gerrit.MockGitiles = mockGitiles

	// Expected branch names by type
	descriptor := "test"
	customBranchName := "new-branch"
	releaseBranchName := fmt.Sprintf("release-R%v-%v-%v.%v.B", expectedBranchVersion.ChromeBranch,
		descriptor, expectedBranchVersion.BuildNumber, expectedBranchVersion.BranchBuildNumber)
	factoryBranchName := fmt.Sprintf("factory-%v-%v.%v.B", descriptor, expectedBranchVersion.BuildNumber,
		expectedBranchVersion.BranchBuildNumber)
	firmwareBranchName := fmt.Sprintf("firmware-%v-%v.%v.B", descriptor, expectedBranchVersion.BuildNumber,
		expectedBranchVersion.BranchBuildNumber)
	stabilizeBranchName := fmt.Sprintf("stabilize-%v-%v.%v.B", descriptor, expectedBranchVersion.BuildNumber,
		expectedBranchVersion.BranchBuildNumber)

	// Mock Download
	manifestFile, err := gerrit.DownloadFileFromGitiles(nil, ctx, "", "manifest", "main", "01/1234.2.0.xml")

	if err != nil {
		t.Error("Error: Failed to download manifest from mock")
		return
	}

	// Create working manifest
	workingManifest, err := ioutil.TempFile("", "working-manifest.xml")
	if err != nil {
		t.Error("Error: working manifest file creation failed")
		return
	}

	// Fill manifest
	_, err = workingManifest.WriteString(manifestFile)
	if err != nil {
		t.Error("Error: Failed write to working manifest")
		return
	}

	// Set exported var with working manifest
	branch.WorkingManifest, err = repo.LoadManifestFromFile(workingManifest.Name())
	if err != nil {
		t.Error("Error: Failed to set brach.WorkingManifest")
		return
	}

	manifestInternal, err := branch.WorkingManifest.GetUniqueProject("chromeos/manifest-internal")
	if err != nil {
		t.Error("Error: Failed to get internal manifest")

		return
	}

	// Validate version
	versionProject, err := branch.WorkingManifest.GetProjectByPath(branch.VersionFileProjectPath)
	if err != nil {
		t.Error("Error: Failed to validate version")
		return
	}

	if versionProject.Path != "src/third_party/chromiumos-overlay" ||
		versionProject.Name != "chromiumos/overlays/chromiumos-overlay" {
		t.Error("Error: version information is incorrect")
		return
	}

	// get chhromeos_version.sh mock
	versionFile, err := gerrit.DownloadFileFromGitiles(nil, ctx, "", "version", "main", "chromeos_version.sh")
	if err != nil {
		t.Error("Error: Failed to download chromeos_version.sh from mock")
		return
	}
	if versionFile != chromeVersionMock {
		t.Error("Error: Downloaded chromeos_version.sh does not match expected")
		return
	}

	// Get parsed version info
	vinfo, err := mv.ParseVersionInfo([]byte(versionFile))

	if err != nil {
		t.Error("Error: Failed to get version info")
		return
	}

	if !mv.VersionsEqual(vinfo, expectedBranchVersion) {
		t.Error("Error: version info does not match expected value")
		return
	}

	// Custom branch type testing (Dry run)
	err = branchCreationTester(manifestInternal, vinfo, customBranchName,
		customBranchName, descriptor, false, false, false, false)
	if err != nil {
		t.Error(err)
	}

	// Release branch type testing (Dry run)
	err = branchCreationTester(manifestInternal, vinfo, releaseBranchName,
		"", descriptor, true, false, false, false)
	if err != nil {
		t.Error(err)
	}

	// Factory branch type testing (Dry run)
	err = branchCreationTester(manifestInternal, vinfo, factoryBranchName,
		"", descriptor, false, true, false, false)
	if err != nil {
		t.Error(err)
	}

	// Firmware branch type testing (Dry run)
	err = branchCreationTester(manifestInternal, vinfo, firmwareBranchName,
		"", descriptor, false, false, true, false)
	if err != nil {
		t.Error(err)
	}

	// Stabilize branch type testing (Dry run)
	err = branchCreationTester(manifestInternal, vinfo, stabilizeBranchName,
		"", descriptor, false, false, false, true)
	if err != nil {
		t.Error(err)
	}
	return
}

func TestCreate(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	manifest := r.Harness.Manifest()
	branch := "new-branch"
	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--file", fullManifestPath(r),
		"--custom", branch,
		"-j", "2", // Test with two workers for kicks.
	})

	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(manifest, branch, ""))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	mainVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("main", mainVersion))

	assertCommentsPersist(t, r, getManifestFiles, branch)
	// Check that manifests were minmally changed (e.g. element ordering preserved).
	// This check is meaningful because the manifests are created using the branch_util
	// tool which reads in, unmarshals, and modifies the manifests from getManifestFiles.
	// The expected manifests (which the branched manifests are being compared to)
	// are simply strings produced by getBranchedManifestFiles.
	assertMinimalManifestChanges(t, r, branch)
}

// Branch off of old-branch to make sure that the source version is being
// bumped in the correct branch.
// Covers crbug.com/1744928.
func TestCreateReleaseNonmain(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	manifest := r.Harness.Manifest()
	branch := "release-R12-2.1.B"
	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--file", fullBranchedManifestPath(r),
		"--release",
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranches([]string{branch}))

	crosFetchVal := manifest.GetRemoteByName("cros").Fetch
	crosInternalFetchVal := manifest.GetRemoteByName("cros-internal").Fetch
	_, _, fullBranchedXML := getBranchedManifestFiles(existingBranchName, crosFetchVal, crosInternalFetchVal)
	var branchManifest *repo.Manifest
	assert.NilError(t, xml.Unmarshal([]byte(fullBranchedXML), &branchManifest))
	branchManifest = branchManifest.ResolveImplicitLinks()

	assert.NilError(t, r.AssertCrosBranchFromManifest(*branchManifest, branch, "old-branch"))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       2,
		BranchBuildNumber: 1,
		PatchNumber:       1,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	sourceVersion := mv.VersionInfo{
		ChromeBranch:      13,
		BuildNumber:       3,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("old-branch", sourceVersion))

	assertCommentsPersist(t, r, getExistingBranchManifestFiles, branch)
}

func TestCreateDryRun(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	branch := "new-branch"
	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"create",
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)
	assertNoRemoteDiff(t, r)
}

// Test creating release branch also bumps main Chrome branch.
func TestCreateRelease(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	manifest := r.Harness.Manifest()

	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--file", fullManifestPath(r),
		"--release",
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	branch := "release-R12-3.B"
	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(manifest, branch, ""))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	mainVersion := mv.VersionInfo{
		ChromeBranch:      13,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("main", mainVersion))

	assertCommentsPersist(t, r, getManifestFiles, branch)
}

// Test create overwrites existing branches when --force is set.
func TestCreateOverwrite(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	manifest := r.Harness.Manifest()

	branch := "old-branch"
	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--force",
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(manifest, branch, ""))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	mainVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("main", mainVersion))

	assertCommentsPersist(t, r, getManifestFiles, branch)
}

// Test create dies when it tries to overwrite without --force.
func TestCreateOverwriteMissingForce(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	manifest := r.Harness.Manifest()

	branch := "old-branch"
	var stderrBuf bytes.Buffer
	stderrLog := log.New(&stderrBuf, "", log.LstdFlags|log.Lmicroseconds)
	s := &branchApplication{application, nil, stderrLog}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret != 0)
	assert.Assert(t, strings.Contains(stderrBuf.String(), "rerun with --force"))

	// Check that no remotes change.
	for _, remote := range manifest.Remotes {
		remotePath := filepath.Join(r.Harness.HarnessRoot(), remote.Name)
		remoteSnapshot, err := r.GetRecentRemoteSnapshot(remote.Name)
		assert.NilError(t, err)
		assert.NilError(t, test_util.AssertContentsEqual(remoteSnapshot, remotePath))
	}
}

// Test create dies when given a version that was already branched.
func TestCreateExistingVersion(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	// Our set up uses branch 12.3.0.0. A branch created from this version must
	// end in 12-3.B. We create a branch with that suffix so that the tool
	// will think 12.3.0.0 has already been branched.
	// We just need to add a branch to the manifest internal repo because
	// the tool checks if a branch exists for a version by looking at
	// branches in the manifest internal repo.
	assert.NilError(t,
		r.Harness.CreateRemoteRef(manifestInternalProject, "release-R12-3.B", ""))
	// Snapshot of manifestInternalProject is stale -- need to update.
	assert.NilError(t, r.TakeSnapshot())

	var stderrBuf bytes.Buffer
	stderrLog := log.New(&stderrBuf, "", log.LstdFlags|log.Lmicroseconds)
	s := &branchApplication{application, nil, stderrLog}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--file", fullManifestPath(r),
		"--stabilize",
	})
	assert.Assert(t, ret != 0)
	assert.Assert(t, strings.Contains(stderrBuf.String(), "already branched 3.0.0"))
	assertNoRemoteDiff(t, r)
}

func TestRename(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_rename")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)
	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	oldBranch := existingBranchName // "old-branch"
	newBranch := "new-branch"

	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"rename", "--push",
		"--manifest-url", manifestDir,
		oldBranch, newBranch,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranches([]string{newBranch}))
	assert.NilError(t, r.AssertCrosBranchesMissing([]string{oldBranch}))

	// Get manifest for oldBranch.
	crosFetchVal := manifest.GetRemoteByName("cros").Fetch
	crosInternalFetchVal := manifest.GetRemoteByName("cros-internal").Fetch
	_, _, fullBranchedXML := getExistingBranchManifestFiles(crosFetchVal, crosInternalFetchVal)
	var branchManifest *repo.Manifest
	assert.NilError(t, xml.Unmarshal([]byte(fullBranchedXML), &branchManifest))
	branchManifest = branchManifest.ResolveImplicitLinks()

	assert.NilError(t, r.AssertCrosBranchFromManifest(*branchManifest, newBranch, oldBranch))
	assertManifestsRepaired(t, r, newBranch)
	newBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       2,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(newBranch, newBranchVersion))

	assertCommentsPersist(t, r, getExistingBranchManifestFiles, newBranch)
	assertMinimalManifestChanges(t, r, newBranch)
}

func TestRenameDryRun(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_rename")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	oldBranch := "old-branch"
	newBranch := "new-branch"

	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"rename",
		"--manifest-url", manifestDir,
		oldBranch, newBranch,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assertNoRemoteDiff(t, r)
}

// Test rename successfully force overwrites.
func TestRenameOverwrite(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_rename")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)
	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	oldBranch := existingBranchName // "old-branch"
	newBranch := "new-branch"

	// Create a branch to rename. To quote the functional tests for `cros branch`:
	// "This may seem like we depend on the correctness of the code under test, but in practice
	// the branches to be renamed will be created by `cros branch` anyways."
	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"create", "--push",
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--custom", newBranch,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranches([]string{newBranch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(manifest, newBranch, ""))
	assertManifestsRepaired(t, r, newBranch)
	newBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(newBranch, newBranchVersion))
	mainVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("main", mainVersion))
	oldBranchVersion := mv.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       2,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(oldBranch, oldBranchVersion))

	assertCommentsPersist(t, r, getManifestFiles, newBranch)

	// Gah! Turns out we actually wanted what's in oldBranch. Let's try force renaming
	// oldBranch to newBranch, overwriting the existing contents of newBranch in the process.
	s = &branchApplication{application, nil, nil}
	ret = subcommands.Run(s, []string{
		"rename", "--push", "--force",
		"--manifest-url", manifestDir,
		oldBranch, newBranch,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranches([]string{newBranch}))
	assert.NilError(t, r.AssertCrosBranchesMissing([]string{oldBranch}))

	// Get manifest for oldBranch.
	crosFetchVal := manifest.GetRemoteByName("cros").Fetch
	crosInternalFetchVal := manifest.GetRemoteByName("cros-internal").Fetch
	_, _, fullBranchedXML := getExistingBranchManifestFiles(crosFetchVal, crosInternalFetchVal)
	var branchManifest *repo.Manifest
	assert.NilError(t, xml.Unmarshal([]byte(fullBranchedXML), &branchManifest))
	branchManifest = branchManifest.ResolveImplicitLinks()

	assert.NilError(t, r.AssertCrosBranchFromManifest(*branchManifest, newBranch, oldBranch))
	assertManifestsRepaired(t, r, newBranch)
	assert.NilError(t, r.AssertCrosVersion(newBranch, oldBranchVersion))

	assertCommentsPersist(t, r, getExistingBranchManifestFiles, newBranch)
}

// Test rename dies if it tries to overwrite without --force.
func TestRenameOverwriteMissingForce(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_rename")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	oldBranch := "old-branch"

	var stderrBuf bytes.Buffer
	stderrLog := log.New(&stderrBuf, "", log.LstdFlags|log.Lmicroseconds)
	s := &branchApplication{application, nil, stderrLog}
	ret := subcommands.Run(s, []string{
		"rename", "--push",
		"--manifest-url", manifestDir,
		"main", oldBranch,
	})
	assert.Assert(t, ret != 0)
	assert.Assert(t, strings.Contains(stderrBuf.String(), "rerun with --force"))
	assertNoRemoteDiff(t, r)
}

func TestDelete(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_delete")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	branchToDelete := "old-branch"

	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	// Some users git cofigs may have init.defaultbranch=main this renames
	// the branch so the test will work properly. Otherwise, this will silently
	// fail and the tests will continue.
	// TODO: Remove once COIL is fully completed
	git.RunGitIgnoreOutput(manifestDir, []string{"branch", "--move", "main", "master"})

	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"delete", "--push", "--force",
		"--manifest-url", manifestDir,
		branchToDelete,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	assert.NilError(t, r.AssertCrosBranchesMissing([]string{branchToDelete}))
}

// Test delete does not modify remote repositories without --push.
func TestDeleteDryRun(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_delete")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	branchToDelete := "old-branch"

	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	// Some users git cofigs may have init.defaultbranch=main this renames
	// the branch so the test will work properly. Otherwise, this will silently
	// fail and the tests will continue.
	// TODO: Remove once COIL is fully completed
	git.RunGitIgnoreOutput(manifestDir, []string{"branch", "--move", "main", "master"})

	s := &branchApplication{application, nil, nil}
	ret := subcommands.Run(s, []string{
		"delete", "--force",
		"--manifest-url", manifestDir,
		branchToDelete,
	})
	assert.Assert(t, ret == 0, "Got return code %d", ret)

	// The test harness has been changed to remove non-inclusive language. This renames the branch
	// to "main" so the test harness can function properly
	// TODO: Remove once COIL is fully completed
	git.RunGitIgnoreOutput(manifestDir, []string{"branch", "--move", "master", "main"})

	assertNoRemoteDiff(t, r)
}

// Test delete does not modify remote when --push set without --force.
func TestDeleteMissingForce(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_delete")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	branchToDelete := "old-branch"

	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)
	var stderrBuf bytes.Buffer
	stderrLog := log.New(&stderrBuf, "", log.LstdFlags|log.Lmicroseconds)
	s := &branchApplication{application, nil, stderrLog}
	ret := subcommands.Run(s, []string{
		"delete", "--push",
		"--manifest-url", manifestDir,
		branchToDelete,
	})
	assert.Assert(t, ret != 0)
	assert.Assert(t, strings.Contains(stderrBuf.String(), "Must set --force to delete remote branches."))
	assertNoRemoteDiff(t, r)
}
