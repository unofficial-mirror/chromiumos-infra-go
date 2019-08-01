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
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	rh "go.chromium.org/chromiumos/infra/go/internal/repo_harness"
	"gotest.tools/assert"
)

var (
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

func getManifestFiles() (manifestFiles, manifestInternalFiles map[string]string, fullTotXML string) {
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

func manifestXml(chunks ...string) string {
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?><manifest>%s</manifest>`,
		strings.Join(chunks, ""))
}

func getDefaultConfig() test.CrosRepoHarnessConfig {
	_, _, fullTotXML := getManifestFiles()

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
	fetchVal := manifest.GetRemoteByName("cros-internal").Fetch
	remoteInternalXML = fmt.Sprintf(remoteInternalXML, fetchVal)
	fetchVal = manifest.GetRemoteByName("cros").Fetch
	remoteExternalXML = fmt.Sprintf(remoteExternalXML, fetchVal)
	manifestFiles, manifestInternalFiles, _ := getManifestFiles()

	// Add manifest files to remote.
	files := []rh.File{}
	for file, contents := range manifestFiles {
		files = append(files, rh.File{
			Name:     file,
			Contents: []byte(contents),
		})
	}
	_, err = r.Harness.AddFiles(manifestProject, "master", files)
	assert.NilError(t, err)

	// Add manifest-internal files to remote.
	files = []rh.File{}
	for file, contents := range manifestInternalFiles {
		files = append(files, rh.File{
			Name:     file,
			Contents: []byte(contents),
		})
	}
	_, err = r.Harness.AddFiles(manifestInternalProject, "master", files)
	assert.NilError(t, err)

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

	manifestFiles, manifestInternalFiles, _ := getManifestFiles()
	assert.NilError(t, r.AssertCrosBranches([]string{branch}))
	assert.NilError(t, r.AssertCrosBranchFromManifest(branch, manifest))
	assert.NilError(t, r.AssertManifestProjectRepaired(
		manifestProject, branch, getKeys(manifestFiles)))
	assert.NilError(t, r.AssertManifestProjectRepaired(
		manifestInternalProject, branch, getKeys(manifestInternalFiles)))
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
