// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
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

func multicheckoutBranchName(project *repo.Project, branch, sourceBranch string) string {
	pid := projectRef(*project)
	// If branch was the result of a rename, then the project ref will start with
	// sourceBranch-. This function removes that bit.
	if sourceBranch != "" {
		pid = strings.Replace(pid, sourceBranch+"-", "", 1)
	}
	branchName := fmt.Sprintf("%s-%s", branch, pid)
	return branchName
}

// versionFileContents returns the contents of a basic ChromeOS version file.
func versionFileContents(version mv.VersionInfo) string {
	contents := fmt.Sprintf("#!/bin/sh\n"+
		"CHROME_BRANCH=%d\nCHROMEOS_BUILD=%d\nCHROMEOS_BRANCH=%d\n,CHROMEOS_PATCH=%d\n",
		version.ChromeBranch, version.BuildNumber, version.BranchBuildNumber, version.PatchNumber)
	return contents
}

// SetVersion sets the version file contents for the specified branch.
// If branch is not set, will use the version project's revision.
func (r *CrosRepoHarness) SetVersion(branch string, version mv.VersionInfo) error {
	if err := r.assertInitialized(); err != nil {
		return err
	}

	if version.VersionFile == "" {
		version.VersionFile = mv.VersionFileProjectPath
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
		if err := r.Harness.AssertProjectBranches(rh.GetRemoteProject(*project), append(branches, "master")); err != nil {
			return err
		}
	}

	multiProjects := manifest.GetMultiCheckoutProjects()
	for _, project := range multiProjects {
		projectBranches := []string{"master"}
		for _, branch := range branches {
			projectBranches = append(projectBranches, multicheckoutBranchName(project, branch, ""))
		}
		if err := r.Harness.AssertProjectBranches(rh.GetRemoteProject(*project), projectBranches); err != nil {
			return err
		}
	}

	pinnedProjects := manifest.GetPinnedProjects()
	for _, project := range pinnedProjects {
		if err := r.Harness.AssertProjectBranches(
			rh.GetRemoteProject(*project), []string{"master", projectRef(*project)}); err != nil {
			return err
		}
	}

	totProjects := manifest.GetTotProjects()
	for _, project := range totProjects {
		if err := r.Harness.AssertProjectBranches(rh.GetRemoteProject(*project), []string{"master"}); err != nil {
			return err
		}
	}

	return nil
}

// AssertCrosBranchesMissing asserts that the specified chromium branch does not exist
// in any projects.
func (r *CrosRepoHarness) AssertCrosBranchesMissing(branches []string) error {
	branchAssertFn := r.Harness.AssertProjectBranchesMissing

	manifest := r.Harness.Manifest()
	singleProjects := manifest.GetSingleCheckoutProjects()
	for _, project := range singleProjects {
		if err := branchAssertFn(rh.GetRemoteProject(*project), append(branches, "master")); err != nil {
			return err
		}
	}

	multiProjects := manifest.GetMultiCheckoutProjects()
	for _, project := range multiProjects {
		projectBranches := []string{"master"}
		for _, branch := range branches {
			projectBranches = append(projectBranches, multicheckoutBranchName(project, branch, ""))
		}
		if err := branchAssertFn(rh.GetRemoteProject(*project), projectBranches); err != nil {
			return err
		}
	}

	// Don't care about pinned/ToT -- nothing would have been created for a particular branch.

	return nil
}

// GetRecentRemoteSnapshot returns the path of the most recent snapshot for a particular remote.
func (r *CrosRepoHarness) GetRecentRemoteSnapshot(remote string) (string, error) {
	remoteSnapshot, ok := r.recentRemoteSnapshots[remote]
	if !ok {
		return "", fmt.Errorf("snapshot does not exist for remote %s", remote)
	}
	return remoteSnapshot, nil
}

func (r *CrosRepoHarness) getRecentProjectSnapshot(project rh.RemoteProject) (string, error) {
	remoteSnapshot, err := r.GetRecentRemoteSnapshot(project.RemoteName)
	if err != nil {
		return "", err
	}
	return filepath.Join(remoteSnapshot, project.ProjectName), nil
}

// AssertCrosBranchFromManifest asserts that the specified CrOS branch descends
// from the given manifest.
// sourceBranch should be set if branch was the result of a branch rename.
func (r *CrosRepoHarness) AssertCrosBranchFromManifest(manifest repo.Manifest, branch string, sourceBranch string) error {
	projectSnapshots := make(map[string]string)
	var err error
	for _, project := range manifest.Projects {
		if projectSnapshots[project.Name], err = r.getRecentProjectSnapshot(rh.GetRemoteProject(project)); err != nil {
			return err
		}
	}

	// For non-pinned/tot projects, check that each project has the revision specified in the manifest
	// as an ancestor.
	singleProjects := manifest.GetSingleCheckoutProjects()
	for _, project := range singleProjects {
		projectSnapshot := projectSnapshots[project.Name]
		err := r.Harness.AssertProjectBranchHasAncestor(
			rh.GetRemoteProject(*project),
			branch,
			projectSnapshot,
			project.Revision)
		if err != nil {
			return err
		}
	}

	multiProjects := manifest.GetMultiCheckoutProjects()
	for _, project := range multiProjects {
		projectSnapshot := projectSnapshots[project.Name]
		err := r.Harness.AssertProjectBranchHasAncestor(
			rh.GetRemoteProject(*project),
			multicheckoutBranchName(project, branch, sourceBranch),
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
			r.Harness.AssertProjectBranchEqual(rh.GetRemoteProject(*project), "master", projectSnapshot),
			r.Harness.AssertProjectBranchEqual(rh.GetRemoteProject(*project), projectRef(*project), projectSnapshot),
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
		if err = r.Harness.AssertProjectBranchEqual(rh.GetRemoteProject(*project), "master", projectSnapshot); err != nil {
			return err
		}
	}

	return nil
}

// AssertCrosVersion asserts that chromeos_version.sh has the expected version numbers.
func (r *CrosRepoHarness) AssertCrosVersion(branch string, version mv.VersionInfo) error {
	if r.versionProject == nil {
		return fmt.Errorf("VersionProject was not set in config")
	}
	if version.VersionFile == "" {
		log.Printf("null version file, using default %s", mv.VersionFileProjectPath)
		version.VersionFile = mv.VersionFileProjectPath
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

	versionInfo, err := mv.ParseVersionInfo(versionFileContents)
	if err != nil {
		return errors.Annotate(err, "could not parse version file %s", version.VersionFile).Err()
	}

	if !mv.VersionsEqual(versionInfo, version) {
		versionInfo.VersionFile = ""
		version.VersionFile = ""
		return fmt.Errorf("version mismatch. expected: %v actual %v", version, versionInfo)
	}

	return nil
}

// AssertNoDefaultRevisions asserts that the given manifest has no default revisions.
func AssertNoDefaultRevisions(manifest repo.Manifest) error {
	if manifest.Default.Revision != "" {
		return fmt.Errorf("manifest <default> has revision %s", manifest.Default.Revision)
	}
	for _, remote := range manifest.Remotes {
		if remote.Revision != "" {
			return fmt.Errorf("<remote> %s has revision %s", remote.Name, remote.Revision)
		}
	}
	return nil
}

func assertEqual(expected, actual string) error {
	if expected != actual {
		return fmt.Errorf("expected: %s got %s", expected, actual)
	}
	return nil
}

func projectInList(project repo.Project, projects []*repo.Project) bool {
	for _, p := range projects {
		if project.Path == p.Path {
			return true
		}
	}
	return false
}

// AssertProjectRevisionsMatchBranch asserts that the project revisions match the given CrOS branch.
func (r *CrosRepoHarness) AssertProjectRevisionsMatchBranch(manifest repo.Manifest, branch, sourceBranch string) error {
	originalManifest := r.Harness.Manifest()
	singleProjects := originalManifest.GetSingleCheckoutProjects()
	multiProjects := originalManifest.GetMultiCheckoutProjects()
	pinnedProjects := originalManifest.GetPinnedProjects()
	totProjects := originalManifest.GetTotProjects()

	for _, project := range manifest.Projects {
		if projectInList(project, singleProjects) {
			if err := assertEqual(git.NormalizeRef(branch), project.Revision); err != nil {
				return errors.Annotate(err, "mismatch for project %s", project.Path).Err()
			}
		}
		if projectInList(project, multiProjects) {
			originalManifest := r.Harness.Manifest()
			originalProject, err := originalManifest.GetProjectByPath(project.Path)
			if err != nil {
				return errors.Annotate(err, "could not get project %s from harness manifest", project.Path).Err()
			}
			expected := git.NormalizeRef(multicheckoutBranchName(originalProject, branch, sourceBranch))
			if err := assertEqual(expected, project.Revision); err != nil {
				return errors.Annotate(err, "mismatch for project %s", project.Path).Err()
			}
		}
		if projectInList(project, pinnedProjects) {
			// Get original revision of project. Make sure that it and the current revision (which will be a SHA)
			// are the same ref.
			originalProject, err := originalManifest.GetProjectByPath(project.Path)
			if err != nil {
				return errors.Annotate(err, "could not get project %s from harness manifest", project.Path).Err()
			}
			pinnedBranch := git.StripRefs(originalProject.Revision)

			projectPath := r.Harness.GetRemotePath(rh.GetRemoteProject(project))
			expected, err := git.GetGitRepoRevision(projectPath, pinnedBranch)
			if err != nil {
				return errors.Annotate(err, "failed to fetch git revision for %s:%s", project.Path, pinnedBranch).Err()
			}
			if err := assertEqual(expected, project.Revision); err != nil {
				return errors.Annotate(err, "mismatch for project %s", project.Path).Err()
			}
		}
		if projectInList(project, totProjects) {
			if err := assertEqual("refs/heads/master", project.Revision); err != nil {
				return errors.Annotate(err, "mismatch for project %s", project.Path).Err()
			}
		}
	}

	return nil
}

// AssertManifestProjectRepaired asserts that the specified manifest XML files in the specified branch
// of a project were repaired.
// This function assumes that r.Harness.SyncLocalCheckout() has just been run.
func (r *CrosRepoHarness) AssertManifestProjectRepaired(
	project rh.RemoteProject, branch string, manifestFiles []string) error {
	tmpDir, err := ioutil.TempDir(r.Harness.HarnessRoot(), "tmp-repo")
	defer os.RemoveAll(tmpDir)
	if err != nil {
		return err
	}

	remotePath := r.Harness.GetRemotePath(project)
	errs := []error{
		git.Clone(remotePath, tmpDir),
		git.Checkout(tmpDir, branch),
	}
	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to checkout branch %s in project %s", branch, project.ProjectName).Err()
		}
	}

	for _, file := range manifestFiles {
		filePath := filepath.Join(tmpDir, file)
		manifest, err := repo.LoadManifestFromFile(filePath)
		if err != nil {
			return errors.Annotate(err, "failed to load manifest file %s", file).Err()
		}
		if err = AssertNoDefaultRevisions(manifest); err != nil {
			return errors.Annotate(err, "manifest %s has error", file).Err()
		}
		if err = r.AssertProjectRevisionsMatchBranch(manifest, branch, ""); err != nil {
			return errors.Annotate(err, "manifest %s has error", file).Err()
		}
	}
	return nil
}

func getComments(file string) []string {
	commentRegex := regexp.MustCompile("<!--.*-->")
	return commentRegex.FindAllString(file, -1)
}

func (r *CrosRepoHarness) AssertCommentsPersist(
	project rh.RemoteProject, branch string, expectedManifestFiles map[string]string) error {
	tmpDir, err := ioutil.TempDir(r.Harness.HarnessRoot(), "tmp-repo")
	defer os.RemoveAll(tmpDir)
	if err != nil {
		return err
	}

	remotePath := r.Harness.GetRemotePath(project)
	errs := []error{
		git.Clone(remotePath, tmpDir),
		git.Checkout(tmpDir, branch),
	}
	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to checkout branch %s in project %s", branch, project.ProjectName).Err()
		}
	}

	for file, expectedContents := range expectedManifestFiles {
		filepath := filepath.Join(tmpDir, file)
		contents, err := ioutil.ReadFile(filepath)
		if err != nil {
			return errors.Annotate(err, "failed to load manifest file %s", file).Err()
		}

		expectedComments := getComments(expectedContents)
		comments := getComments(string(contents))

		if !reflect.DeepEqual(expectedComments, comments) {
			return fmt.Errorf("Comment mismatch. Expected %v got %v", expectedComments, comments)
		}
	}
	return nil
}
