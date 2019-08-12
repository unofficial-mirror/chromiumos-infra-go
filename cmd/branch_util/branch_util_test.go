// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/cmd/branch_util/test"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	rh "go.chromium.org/chromiumos/infra/go/internal/repo_harness"
	"go.chromium.org/chromiumos/infra/go/internal/test_util"
	"gotest.tools/assert"
)

const (
	defaultFileName  = "default.xml"
	officialFileName = "official.xml"
	externalFileName = "external.xml"
	internalFileName = "internal.xml"
	remotesFileName  = "_remotes.xml"

	defaultXML = `
	<default revision="refs/heads/master" remote="cros"/>
`
	remoteExternalXML = `
  <remote name="cros" revision="refs/heads/master" fetch="%s"/>
`
	remoteInternalXML = `
  <remote name="cros-internal" revision="refs/heads/master" fetch="%s"/>
`
	projectsExternalXML = `
  <project name="chromiumos/manifest" path="manifest"/>

  <project name="chromiumos/overlays/chromiumos-overlay"
           path="src/third_party/chromiumos-overlay"/>

  <project name="external/implicit-pinned"
           path="src/third_party/implicit-pinned"
           revision="refs/heads/implicit-pinned"/>

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
           remote="cros-internal"/>

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
	defaultBranchedXML = `
  <default remote="cros"/>
`
	projectsExternalBranchedXML = `
  <project name="chromiumos/manifest"
           path="manifest"
           revision="refs/heads/old-branch"/>

  <project name="chromiumos/overlays/chromiumos-overlay"
           path="src/third_party/chromiumos-overlay"
           revision="refs/heads/old-branch"/>

  <project name="external/implicit-pinned"
           path="src/third_party/implicit-pinned"
           revision="refs/heads/implicit-pinned"/>

  <project name="chromiumos/multicheckout"
           path="src/third_party/multicheckout-a"
           revision="refs/heads/old-branch-multicheckout-a"/>

  <project name="chromiumos/multicheckout"
           path="src/third_party/multicheckout-b"
           revision="refs/heads/old-branch-multicheckout-b"/>
`

	projectsInternalBranchedXML = `
  <project name="chromeos/manifest-internal"
           path="manifest-internal"
           remote="cros-internal"
           revision="refs/heads/old-branch"/>

  <project name="chromeos/explicit-pinned"
           path="src/explicit-pinned"
           revision="refs/heads/explicit-pinned"
           remote="cros-internal">
    <annotation name="branch-mode" value="pin"/>
  </project>

  <project name="chromeos/explicit-branch"
           path="src/explicit-branch"
           remote="cros-internal"
           revision="refs/heads/old-branch">
    <annotation name="branch-mode" value="create"/>
  </project>

  <project name="chromeos/explicit-tot"
           path="src/explicit-tot"
           remote="cros-internal"
           revision="refs/heads/master">
    <annotation name="branch-mode" value="tot"/>
  </project>
`
)

var (
	manifestProject = rh.RemoteProject{
		RemoteName:  "cros",
		ProjectName: "chromiumos/manifest",
	}
	manifestInternalProject = rh.RemoteProject{
		RemoteName:  "cros-internal",
		ProjectName: "chromeos/manifest-internal",
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

func getBranchedManifestFiles(crosFetch, crosInternalFetch string) (
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
	remoteInternalXML := fmt.Sprintf(remoteInternalXML, crosInternalFetch)
	remoteExternalXML := fmt.Sprintf(remoteExternalXML, crosFetch)

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

func manifestXml(chunks ...string) string {
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?><manifest>%s</manifest>`,
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

func runCommand(cmd []string, stdoutBuf, stderrBuf *bytes.Buffer) error {
	ctx := context.Background()
	cmdCtx := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	cmdCtx.Stdout = stdoutBuf
	cmdCtx.Stderr = stderrBuf
	cmdCtx.Dir, _ = filepath.Abs("/usr/local/google/home/jackneus/chromiumos/chromite")
	fmt.Printf("%s\n", strings.Join(cmd, " "))
	return cmdCtx.Run()
}

func fullManifestPath(r *test.CrosRepoHarness) string {
	return filepath.Join(r.Harness.HarnessRoot(), "manifest.xml")
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
	version := repo.VersionInfo{
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
		getBranchedManifestFiles(crosFetchVal, crosInternalFetchVal)

	// Add manifest files to remote.
	addManifestFiles(t, &r, manifestProject, "master", manifestFiles)

	// Add manifest-internal files to remote.
	addManifestFiles(t, &r, manifestInternalProject, "master", manifestInternalFiles)

	// Create existing branch on remote.
	branchName := "old-branch"
	var branchManifest *repo.Manifest
	assert.NilError(t, xml.Unmarshal([]byte(fullBranchedXML), &branchManifest))
	branchManifest = branchManifest.ResolveImplicitLinks()

	// Create ref for each project.
	for _, project := range branchManifest.Projects {
		projectBranch := git.StripRefs(project.Revision)
		err = r.Harness.CreateRemoteRef(rh.GetRemoteProject(project), projectBranch, "")
		if err != nil && strings.Contains(err.Error(), "already exists") {
			continue
		}
		assert.NilError(t, err)
	}
	// Set version file.
	version = repo.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       2,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.SetVersion(branchName, version))
	// Commit manifest files.
	addManifestFiles(t, &r, manifestProject, branchName, manifestBranchedFiles)
	addManifestFiles(t, &r, manifestInternalProject, branchName, manifestInternalBranchedFiles)

	// Sync local checkout to get manifest.
	assert.NilError(t, r.Harness.SyncLocalCheckout())
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

func TestCreate(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_create")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	branch := "new-branch"
	s := &branchApplication{application, log.New(ioutil.Discard, "", log.LstdFlags|log.Lmicroseconds)}
	ret := subcommands.Run(s, []string{
		"create", "--push", "--root", localRoot,
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret == 0)

	// Sync local checkout before asserts.
	assert.NilError(t, r.Harness.SyncLocalCheckout())

	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(branch, manifest))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := repo.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	masterVersion := repo.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("master", masterVersion))
}

func TestCreateDryRun(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_create")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	branch := "new-branch"
	s := &branchApplication{application, log.New(ioutil.Discard, "", log.LstdFlags|log.Lmicroseconds)}
	ret := subcommands.Run(s, []string{
		"create", "--root", localRoot,
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret == 0)

	// Check that no remotes change.
	for _, remote := range manifest.Remotes {
		remotePath := filepath.Join(r.Harness.HarnessRoot(), remote.Name)
		remoteSnapshot, err := r.GetRecentRemoteSnapshot(remote.Name)
		assert.NilError(t, err)
		assert.NilError(t, test_util.AssertContentsEqual(remoteSnapshot, remotePath))
	}
}

// Test creating release branch also bumps master Chrome branch.
func TestCreateRelease(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_create")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	s := &branchApplication{application, log.New(ioutil.Discard, "", log.LstdFlags|log.Lmicroseconds)}
	ret := subcommands.Run(s, []string{
		"create", "--push", "--root", localRoot,
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--release",
	})
	assert.Assert(t, ret == 0)

	// Sync local checkout before asserts.
	assert.NilError(t, r.Harness.SyncLocalCheckout())

	branch := "release-R12-3.B"
	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(branch, manifest))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := repo.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	masterVersion := repo.VersionInfo{
		ChromeBranch:      13,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("master", masterVersion))
}

// Test create overwrites existing branches when --force is set.
func TestCreateOverwrite(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_create")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	branch := "old-branch"
	s := &branchApplication{application, log.New(ioutil.Discard, "", log.LstdFlags|log.Lmicroseconds)}
	ret := subcommands.Run(s, []string{
		"create", "--push", "--root", localRoot,
		"--force",
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret == 0)

	// Sync local checkout before asserts.
	assert.NilError(t, r.Harness.SyncLocalCheckout())

	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(branch, manifest))
	assertManifestsRepaired(t, r, branch)
	newBranchVersion := repo.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       3,
		BranchBuildNumber: 1,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion(branch, newBranchVersion))
	masterVersion := repo.VersionInfo{
		ChromeBranch:      12,
		BuildNumber:       4,
		BranchBuildNumber: 0,
		PatchNumber:       0,
	}
	assert.NilError(t, r.AssertCrosVersion("master", masterVersion))
}

// Test create dies when it tries to overwrite without --force.
func TestCreateOverwriteMissingForce(t *testing.T) {
	r := setUp(t)
	defer r.Teardown()

	localRoot, err := ioutil.TempDir("", "test_create")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	branch := "old-branch"
	var stdoutBuf bytes.Buffer
	branchLog := log.New(&stdoutBuf, "", log.LstdFlags|log.Lmicroseconds)
	s := &branchApplication{application, branchLog}
	ret := subcommands.Run(s, []string{
		"create", "--push", "--root", localRoot,
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--custom", branch,
	})
	assert.Assert(t, ret != 0)
	// TODO(@jackneus): fix logging so that we can make this assert.
	//assert.Assert(t, strings.Contains(stdoutBuf.String(), "rerun with --force"))

	// Sync local checkout before asserts.
	assert.NilError(t, r.Harness.SyncLocalCheckout())

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

	localRoot, err := ioutil.TempDir("", "test_create")
	defer os.RemoveAll(localRoot)
	assert.NilError(t, err)

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

	manifest := r.Harness.Manifest()
	manifestDir := r.Harness.GetRemotePath(manifestInternalProject)

	s := &branchApplication{application, log.New(ioutil.Discard, "", log.LstdFlags|log.Lmicroseconds)}
	ret := subcommands.Run(s, []string{
		"create", "--push", "--root", localRoot,
		"--manifest-url", manifestDir,
		"--file", fullManifestPath(r),
		"--stabilize",
	})
	assert.Assert(t, ret != 0)

	// TODO(@jackneus): fix logging so that we can make this assert.
	//assert.Assert(t, strings.Contains(stdoutBuf.String(), "already branched 3.0.0"))

	// Check that no remotes change.
	for _, remote := range manifest.Remotes {
		remotePath := filepath.Join(r.Harness.HarnessRoot(), remote.Name)
		remoteSnapshot, err := r.GetRecentRemoteSnapshot(remote.Name)
		assert.NilError(t, err)
		assert.NilError(t, test_util.AssertContentsEqual(remoteSnapshot, remotePath))
	}
}
