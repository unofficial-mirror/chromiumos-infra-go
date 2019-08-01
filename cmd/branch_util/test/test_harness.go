// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test

import (
	"fmt"
	"log"
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
	DefaultVersionProject = repo.Project{
		Name:       "chromiumos-overlay/overlays/chromiumos-overlay",
		Path:       "src/third_party/chromiumos-overlay",
		RemoteName: RemoteCros,
	}
	DefaultManifestProject = repo.Project{
		Name:       "chromiumos/" + ProjectManifest,
		Path:       ProjectManifest,
		RemoteName: RemoteCros,
	}
	DefaultManifestInternalProject = repo.Project{
		Name:       "chromiumos/" + ProjectManifestInternal,
		Path:       ProjectManifestInternal,
		RemoteName: RemoteCrosInternal,
	}
	DefaultProjects = []repo.Project{
		// Version file project.
		DefaultVersionProject,
		// Manifest projects.
		DefaultManifestProject,
		DefaultManifestInternalProject,
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
		VersionProject: DefaultVersionProject.Name,
	}
)

type CrosRepoHarness struct {
	Harness rh.RepoHarness
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

	err := r.Harness.Initialize(&rh.RepoHarnessConfig{
		Manifest: config.Manifest,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *CrosRepoHarness) Teardown() error {
	return r.Harness.Teardown()
}

func (r *CrosRepoHarness) assertInitialized() error {
	if r.Harness.HarnessRoot() == "" {
		return fmt.Errorf("harness needs to be initialized")
	}
	return nil
}

func projectRef(project repo.Project) string {
	if project.Upstream != "" {
		return git.StripRefs(project.Upstream)
	} else {
		return git.StripRefs(project.Revision)
	}
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
	_, err := r.Harness.AddFile(rh.GetRemoteProject(*r.versionProject), branch, versionFile)
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
	for _, remote := range r.Harness.Manifest().Remotes {
		remotePath := filepath.Join(r.Harness.HarnessRoot(), remote.Name)
		var err error
		r.recentRemoteSnapshots[remote.Name], err = r.Harness.Snapshot(remotePath)
		if err != nil {
			return errors.Annotate(err, "error taking snapshot of remote %s", remote.Name).Err()
		}
	}

	return nil
}

// AssertCrosBranches asserts that remote projects have the expected chromiumos branches.
func (r *CrosRepoHarness) AssertCrosBranches(branches []string) error {
	manifest := r.Harness.Manifest()
	singleProjects := manifest.GetSingleCheckoutProjects()
	for _, project := range singleProjects {
		if err := r.Harness.AssertProjectBranches(rh.GetRemoteProject(project), append(branches, "master")); err != nil {
			return err
		}
	}

	multiProjects := manifest.GetMultiCheckoutProjects()
	for _, project := range multiProjects {
		projectBranches := []string{"master"}
		pid := projectRef(project)
		for _, branch := range branches {
			projectBranches = append(projectBranches, fmt.Sprintf("%s-%s", branch, pid))
		}
		if err := r.Harness.AssertProjectBranches(rh.GetRemoteProject(project), projectBranches); err != nil {
			return err
		}
	}

	pinnedProjects := manifest.GetPinnedProjects()
	for _, project := range pinnedProjects {
		if err := r.Harness.AssertProjectBranches(
			rh.GetRemoteProject(project), []string{"master", projectRef(project)}); err != nil {
			return err
		}
	}

	totProjects := manifest.GetTotProjects()
	for _, project := range totProjects {
		if err := r.Harness.AssertProjectBranches(rh.GetRemoteProject(project), []string{"master"}); err != nil {
			return err
		}
	}

	return nil
}

func (r *CrosRepoHarness) getInitialProjectSnapshot(project rh.RemoteProject) (string, error) {
	remoteSnapshot, ok := r.recentRemoteSnapshots[project.RemoteName]
	if !ok {
		return "", fmt.Errorf("snapshot does not exist for remote %s", project.RemoteName)
	}
	return filepath.Join(remoteSnapshot, project.ProjectName), nil
}

// AssertCrosBranchFromManifest asserts that the specified CrOS branch descends
// from the given manifest.
func (r *CrosRepoHarness) AssertCrosBranchFromManifest(branch string, manifest repo.Manifest) error {
	projectSnapshots := make(map[string]string)
	var err error
	for _, project := range manifest.Projects {
		if projectSnapshots[project.Name], err = r.getInitialProjectSnapshot(rh.GetRemoteProject(project)); err != nil {
			return err
		}
	}

	// For non-pinned/tot projects, check that each project has the revision specified in the manifest
	// as an ancestor.
	singleProjects := manifest.GetSingleCheckoutProjects()
	for _, project := range singleProjects {
		projectSnapshot := projectSnapshots[project.Name]
		err := r.Harness.AssertProjectBranchHasAncestor(
			rh.GetRemoteProject(project),
			branch,
			projectSnapshot,
			project.Revision)
		if err != nil {
			return err
		}
	}

	multiProjects := manifest.GetMultiCheckoutProjects()
	for _, project := range multiProjects {
		pid := projectRef(project)
		projectSnapshot := projectSnapshots[project.Name]
		err := r.Harness.AssertProjectBranchHasAncestor(
			rh.GetRemoteProject(project),
			fmt.Sprintf("%s-%s", branch, pid),
			projectSnapshot, project.Revision)
		if err != nil {
			return err
		}
	}

	// For pinned/tot projects, check that each project is unchanged.
	pinnedProjects := manifest.GetPinnedProjects()
	for _, project := range pinnedProjects {
		projectSnapshot := projectSnapshots[project.Name]
		errs := []error{
			r.Harness.AssertProjectBranchEqual(rh.GetRemoteProject(project), "master", projectSnapshot),
			r.Harness.AssertProjectBranchEqual(rh.GetRemoteProject(project), projectRef(project), projectSnapshot),
		}
		for _, err = range errs {
			if err != nil {
				return err
			}
		}
	}

	totProjects := manifest.GetTotProjects()
	for _, project := range totProjects {
		projectSnapshot := projectSnapshots[project.Name]
		if err = r.Harness.AssertProjectBranchEqual(rh.GetRemoteProject(project), "master", projectSnapshot); err != nil {
			return err
		}
	}

	return nil
}

// AssertCrosVersion asserts that chromeos_version.sh has the expected version numbers.
func (r *CrosRepoHarness) AssertCrosVersion(branch string, version repo.VersionInfo) error {
	if r.versionProject == nil {
		return fmt.Errorf("VersionProject was not set in config")
	}
	if version.VersionFile == "" {
		log.Printf("null version file, using default %s", repo.VersionFileProjectPath)
		version.VersionFile = repo.VersionFileProjectPath
	}
	manifest := r.Harness.Manifest()
	project, err := manifest.GetProjectByName(r.versionProject.Name)
	if err != nil {
		return errors.Annotate(err, "error getting chromeos version project %s", project.Name).Err()
	}
	versionFileContents, err := r.Harness.ReadFile(rh.GetRemoteProject(*project), branch, version.VersionFile)
	if err != nil {
		return errors.Annotate(err, "could not read version file %s", version.VersionFile).Err()
	}

	versionInfo, err := repo.ParseVersionInfo(versionFileContents)
	if err != nil {
		return errors.Annotate(err, "could not parse version file %s", version.VersionFile).Err()
	}

	if !repo.VersionsEqual(versionInfo, version) {
		versionInfo.VersionFile = ""
		version.VersionFile = ""
		return fmt.Errorf("version mismatch. expected: %v actual %v", version, versionInfo)
	}

	return nil
}
