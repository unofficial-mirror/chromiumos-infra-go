// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test

import (
	"fmt"
	"path/filepath"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	rh "go.chromium.org/chromiumos/infra/go/internal/repo_harness"
	"go.chromium.org/luci/common/errors"
)

// This is intended to be a more specific version of RepoHarness
// that caters to the specific setup of the ChromeOS project.

const (
	RemoteCros              = "cros"
	RemoteCrosInternal      = "cros-internal"
	ProjectManifest         = "manifest"
	ProjectManifestInternal = "manifest-internal"
)

var (
	DefaultRemotes = []repo.Remote{
		{Name: RemoteCros},
		{Name: RemoteCrosInternal},
	}
	defaultVersionProject = repo.Project{
		Name:       "chromiumos-overlay/overlays/chromiumos-overlay",
		Path:       "src/third_party/chromiumos-overlay",
		RemoteName: RemoteCros,
	}
	defaultManifestProject = repo.Project{
		Name:       "chromiumos/" + ProjectManifest,
		Path:       ProjectManifest,
		RemoteName: RemoteCros,
	}
	defaultManifestInternalProject = repo.Project{
		Name:       "chromiumos/" + ProjectManifestInternal,
		Path:       ProjectManifestInternal,
		RemoteName: RemoteCrosInternal,
	}
	DefaultProjects = []repo.Project{
		// Version file project.
		defaultVersionProject,
		// Manifest projects.
		defaultManifestProject,
		defaultManifestInternalProject,
	}

	// Default config for a CrOS repo harness.
	DefaultCrosHarnessConfig = CrosRepoHarnessConfig{
		Manifest: repo.Manifest{
			Projects: DefaultProjects,
			Remotes:  DefaultRemotes,
			Default: repo.Default{
				RemoteName: RemoteCros,
				Revision:   "refs/heads/master",
			},
		},
		VersionProject: defaultVersionProject.Name,
	}
)

type CrosRepoHarness struct {
	harness rh.RepoHarness
	// Version info project information.
	versionProject *repo.Project

	// Snapshots of each remote taken after initialization.
	recentRemoteSnapshots map[string]string
}

type CrosRepoHarnessConfig struct {
	// Initialize() will create a test harness with
	// the appropriate remote repos and a local repo.
	// Both remote and local repos will have the appropriate
	// projects created (with initialized git repos inside them).
	Manifest repo.Manifest
	// Version info project name. Should exist in Manifest.
	VersionProject string
}

func (r *CrosRepoHarness) Initialize(config *CrosRepoHarnessConfig) error {
	if config.VersionProject == "" {
		return fmt.Errorf("version project not specified")
	}
	// If VersionProject is set, check that it is in the manifest.
	foundVersionProject := false
	for i := range config.Manifest.Projects {
		if config.VersionProject == config.Manifest.Projects[i].Name {
			r.versionProject = &config.Manifest.Projects[i]
			foundVersionProject = true
			break
		}
	}
	if !foundVersionProject {
		return fmt.Errorf("version project %v does not exist in specified manifest", config.VersionProject)
	}

	err := r.harness.Initialize(&rh.RepoHarnessConfig{
		Manifest: config.Manifest,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *CrosRepoHarness) Teardown() error {
	return r.harness.Teardown()
}

func (r *CrosRepoHarness) assertInitialized() error {
	if r.harness.HarnessRoot() == "" {
		return fmt.Errorf("harness needs to be initialized")
	}
	return nil
}

// versionFileContents returns the contents of a basic ChromeOS version file.
func versionFileContents(version repo.VersionInfo) string {
	contents := fmt.Sprintf("#!/bin/sh\n"+
		"CHROME_BRANCH=%d\nCHROMEOS_BUILD=%d\nCHROMEOS_BRANCH=%d\n,CHROMEOS_PATCH=%d\n",
		version.ChromeBranch, version.BuildNumber, version.BranchBuildNumber, version.PatchNumber)
	return contents
}

// SetVersion sets the version file contents for the specified branch.
// If branch is not set, will use the version project's revision.
func (r *CrosRepoHarness) SetVersion(branch string, version repo.VersionInfo) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}

	if version.VersionFile == "" {
		version.VersionFile = repo.VersionFileProjectPath
	}
	versionFile := rh.File{
		Name:     version.VersionFile,
		Contents: []byte(versionFileContents(version)),
	}
	if branch == "" {
		branch = git.StripRefs(r.versionProject.Revision)
	}
	_, err := r.harness.AddFile(rh.GetRemoteProject(*r.versionProject), branch, versionFile)
	if err != nil {
		return errors.Annotate(err, "failed to add version file").Err()
	}
	return nil
}

// TakeSnapshot takes a snapshot of the current state of each remote and stores them
// within the harness struct.
func (r *CrosRepoHarness) TakeSnapshot() error {
	// Take snapshot of each project in its current state.
	r.recentRemoteSnapshots = make(map[string]string)
	for _, remote := range r.harness.Manifest().Remotes {
		remotePath := filepath.Join(r.harness.HarnessRoot(), remote.Name)
		var err error
		r.recentRemoteSnapshots[remote.Name], err = r.harness.Snapshot(remotePath)
		if err != nil {
			return errors.Annotate(err, "error taking snapshot of remote %s", remote.Name).Err()
		}
	}
	return nil
}
