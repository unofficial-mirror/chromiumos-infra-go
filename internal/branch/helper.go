// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package branch

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/chromiumos/infra/go/internal/shared"
	"go.chromium.org/luci/common/errors"
)

const (
	gitRetries = 3
	gitTimeout = 30 * time.Second
)

var (
	ManifestProjects = map[string]bool{
		"chromiumos/manifest":        true,
		"chromeos/manifest-internal": true,
	}
	// A Chrome OS branch name as a prefix to a string, starting with a dash.
	// e.g. -release-R77-12371.B
	BranchPrefix = regexp.MustCompile("^-.*[.]B")
)

// ProjectBranch is a project and a branch.
type ProjectBranch struct {
	Project    repo.Project
	BranchName string
}

// branchMapEntry is a struct for manual branch mapping
type branchMapEntry struct {
	name        string
	path        string
	suffix      string
	replacement string
}

// branchMap is a map that defines branch suffix replacements
var branchMap = map[string]branchMapEntry{
	"chromiumos/third_party/coreboot": {
		name:        "chromiumos/third_party/coreboot",
		path:        "src/third_party/coreboot",
		suffix:      "-chromeos-2016.05",
		replacement: "",
	},
}

// canBranchProject retuns true if the Project can be branched.
func canBranchProject(manifest repo.Manifest, project repo.Project) bool {
	return manifest.ProjectBranchMode(project) == repo.Create
}

// projectBranchName determines the git branch name for the Project.
func projectBranchName(br string, project repo.Project, original string) string {
	// If the Project has only one checkout, then the base branch name is fine.
	var checkouts []string
	for _, proj := range WorkingManifest.Projects {
		if proj.Name == project.Name {
			checkouts = append(checkouts, proj.Name)
		}
	}

	if len(checkouts) == 1 {
		return br
	}

	// Otherwise, the project name needs a suffix. We append its upstream or
	// revision to distinguish it from other checkouts. We grab the suffix
	// from git using the Upstream or Revision. We then trim and replace
	// any unneeded info from the suffix.
	suffix := "-"
	if project.Upstream != "" {
		suffix += git.StripRefs(project.Upstream)
	} else {
		suffix += git.StripRefs(project.Revision)
	}

	// If the revision is itself a branch, we need to strip the old branch name
	// from the suffix to keep naming consistent.
	if original != "" {
		if strings.HasPrefix(suffix, "-"+original+"-") {
			suffix = strings.TrimPrefix(suffix, "-"+original)
		}
	}
	if BranchPrefix.MatchString(suffix) {
		// If the suffix already has a version in it, trim that.
		// e.g. -release-R77-12371.B-wpa_supplicant-2.6 --> -wpa_supplicant-2.6
		suffix = BranchPrefix.ReplaceAllString(suffix, "")
	}
	// Remove the "/" character, since those don't belong in branch names.
	suffix = strings.ReplaceAll(suffix, "/", "-")

	// Check if a branch naming override exist
	entry, ok := branchMap[project.Name]

	// Replace suffix
	if ok && (suffix == entry.suffix && project.Path == entry.path) {
		suffix = entry.replacement
	}

	return br + suffix
}

// ProjectBranches returns a list of ProjectBranch structs:
// one for each branchable project.
// The original parameter is the CrOS branch from which the current checkout stems.
func ProjectBranches(br, original string) []ProjectBranch {
	var projectBranches []ProjectBranch
	for _, project := range WorkingManifest.Projects {
		if canBranchProject(WorkingManifest, project) {
			projectBranches = append(projectBranches,
				ProjectBranch{
					Project:    project,
					BranchName: projectBranchName(br, project, original),
				})
		}
	}
	return projectBranches
}

// BranchExists checks that a branch matching the given pattern exists in a particular project.
func BranchExists(branchPattern *regexp.Regexp, buildNumber string, branchType string, remoteBranches []string) (bool, error) {
	for _, branch := range remoteBranches {
		// Failing cases
		switch {
		case branchPattern.Match([]byte(branch)):
			return true, nil
		case strings.Contains(branch, buildNumber):

			// Check that major version collision is on other branch type
			if !strings.Contains(branch, branchType) {
				err := errors.New("ERROR: Major version collision on branch " + branch)
				return true, err
			}
		}
	}

	return false, nil
}

// branchExistsExplicit checks that the given branch exists in the project.
// It is a good bit faster than BranchExists.
func branchExistsExplicit(project repo.Project, br string) (bool, error) {
	remoteUrl, err := ProjectFetchUrl(project.Path)
	if err != nil {
		return false, errors.Annotate(err, "failed to get remote project url").Err()
	}

	ch := make(chan bool, 1)

	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	opts := shared.DefaultOpts
	opts.Retries = gitRetries
	err = shared.DoWithRetry(ctx, opts, func() error {
		// If we give a full URL, don't need to run the command in a git repo.
		output, err := git.RemoteHasBranch("", remoteUrl, br)
		if err != nil {
			return err
		}
		ch <- output
		return nil
	})
	if err != nil {
		return false, errors.Annotate(err, "failed to list remote branches for %s", remoteUrl).Err()
	}

	return <-ch, nil
}

func assertBranchesDoNotExistWorker(
	wg *sync.WaitGroup, projectBranches <-chan ProjectBranch, errs chan<- error) {
	for projectBranch := range projectBranches {
		LogOut("...checking that %s does not exist in %s.\n",
			projectBranch.BranchName,
			projectBranch.Project.Name)
		exists, err := branchExistsExplicit(projectBranch.Project, projectBranch.BranchName)
		if err == nil {
			if exists {
				errs <- fmt.Errorf("branch %s exists for %s. Please rerun with --force to proceed.",
					projectBranch.BranchName, projectBranch.Project.Name)
			}
		} else {
			errs <- err
		}
		wg.Done()
	}
}

// AssertBranchesDoNotExist checks that branches do not already exist.
func AssertBranchesDoNotExist(branches []ProjectBranch, workerCount int) error {
	projectBranches := make(chan ProjectBranch, len(branches))
	errs := make(chan error, len(branches))

	var wg sync.WaitGroup
	for i := 1; i <= workerCount; i++ {
		go assertBranchesDoNotExistWorker(&wg, projectBranches, errs)
	}

	for _, projectBranch := range branches {
		projectBranches <- projectBranch
		wg.Add(1)
	}
	close(projectBranches)

	wg.Wait()

	select {
	case err := <-errs:
		return err
	default:
	}
	return nil
}

// GerritProjectBranches creates a slice of GerritProjectBranch objects, which
// are representations of ProjectBranches that are useful for API based
// branching.
func GerritProjectBranches(pbs []ProjectBranch) ([]GerritProjectBranch, error) {
	var result []GerritProjectBranch
	for _, pb := range pbs {
		remote := WorkingManifest.GetRemoteByName(pb.Project.RemoteName)
		if remote == nil {
			return result, fmt.Errorf("remote %s does not exist in working manifest", pb.Project.RemoteName)
		}
		remoteURL, err := url.Parse(remote.Fetch)
		if err != nil {
			return result, errors.Annotate(err, "failed to parse fetch location for remote %s", remote.Name).Err()
		}
		remoteStr := remoteURL.String()
		remoteStr = strings.ReplaceAll(remoteStr,
			"chromium.googlesource.com", "chromium-review.googlesource.com")
		remoteStr = strings.ReplaceAll(remoteStr,
			"chrome-internal.googlesource.com", "chrome-internal-review.googlesource.com")
		result = append(result, GerritProjectBranch{
			GerritURL: remoteStr,
			Project:   pb.Project.Name,
			Branch:    pb.BranchName,
			SrcRef:    pb.Project.Revision,
		})
	}
	return result, nil
}

// getBranchesByPath returns a map mapping project paths to git branch names.
func getBranchesByPath(branches []ProjectBranch) map[string]string {
	branchesByPath := make(map[string]string)
	for _, branch := range branches {
		branchesByPath[branch.Project.Path] = branch.BranchName
	}
	return branchesByPath
}

// getOriginRef returns the equivalent of the specified Ref for the "origin" remote.
func getOriginRef(ref string) string {
	// If the revision starts with refs/heads/, make it start with
	// refs/remotes/origin instead.
	if git.StripRefsHead(ref) != ref {
		return "refs/remotes/origin/" + git.StripRefsHead(ref)
	}
	// If the revision is a SHA, let it be.
	return ref
}

// GetNonManifestBranches filters out non-Manifest branches.
func GetNonManifestBranches(branches []GerritProjectBranch) []GerritProjectBranch {
	var result []GerritProjectBranch
	for _, pb := range branches {
		if _, ok := ManifestProjects[pb.Project]; ok {
			continue
		}
		result = append(result, pb)
	}
	return result
}

// RepairManifestRepositories repairs all manifests in all manifest repositories
// on the current branch and commits the changes. It then pushes the state of
// the local git branches to remote.
func RepairManifestRepositories(branches []ProjectBranch, dryRun, force bool) error {
	manifestBranchNames := make(map[string]string)

	// Find names of manifest project branches so that we can push changes.
	for _, projectBranch := range branches {
		if _, ok := ManifestProjects[projectBranch.Project.Name]; ok {
			manifestBranchNames[projectBranch.Project.Name] = projectBranch.BranchName
		}
	}

	for projectName := range ManifestProjects {
		manifestProject, err := WorkingManifest.GetUniqueProject(projectName)
		if err != nil {
			return err
		}
		opts := &CheckoutOptions{
			Depth: 1,
			Ref:   manifestProject.Revision,
		}
		manifestCheckout, err := GetProjectCheckout(manifestProject.Path, opts)

		defer os.RemoveAll(manifestCheckout)

		if err != nil {
			return errors.Annotate(err, "failed to checkout project %s", manifestProject.Path).Err()
		}

		manifestRepo := ManifestRepo{
			ProjectCheckout: manifestCheckout,
			Project:         manifestProject,
		}
		if err := manifestRepo.RepairManifestsOnDisk(getBranchesByPath(branches)); err != nil {
			return errors.Annotate(err, "failed to repair manifest project %s", projectName).Err()
		}
		if output, err := git.RunGit(manifestCheckout,
			[]string{"commit", "-a", "-m", "commit repaired manifests"}); err != nil {
			return fmt.Errorf("error committing repaired manifests: %s", output.Stdout)
		}

		remoteRef := git.RemoteRef{
			Remote: "origin",
			Ref:    git.NormalizeRef(manifestBranchNames[manifestProject.Name]),
		}
		gitOpts := git.GitOpts{
			DryRun: dryRun,
			Force:  force,
		}
		if err := git.PushRef(manifestCheckout, "HEAD", remoteRef, gitOpts); err != nil {
			return errors.Annotate(err, "could not push branches to remote").Err()
		}
	}
	return nil
}

func createRemoteBranchesWorker(
	wg *sync.WaitGroup,
	branches <-chan ProjectBranch,
	errs chan<- error,
	dryRun, force bool) {
	for projectBranch := range branches {
		opts := &CheckoutOptions{
			Depth: 1,
			Ref:   projectBranch.Project.Revision,
		}
		projectCheckout, err := GetProjectCheckout(projectBranch.Project.Path, opts)
		defer os.RemoveAll(projectCheckout)
		if err != nil {
			errs <- errors.Annotate(err, "could not checkout %s:%s",
				projectBranch.Project.Path, projectBranch.BranchName).Err()
			wg.Done()
			continue
		}

		branchName := git.NormalizeRef(projectBranch.BranchName)
		refspec := fmt.Sprintf("%s:%s", getOriginRef(projectBranch.Project.Revision), branchName)

		logMode := "Pushing"
		cmd := []string{"push", "origin", refspec}
		if dryRun {
			cmd = append(cmd, "--dry-run")
			logMode = "Dry run"
		}
		if force {
			cmd = append(cmd, "--force")
			logMode += " (with --force flag)"
		}
		LogOut("%s ref %s for project %s\n", logMode, branchName, projectBranch.Project.Path)

		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()
		retryOpts := shared.DefaultOpts
		retryOpts.Retries = gitRetries
		err = shared.DoWithRetry(ctx, retryOpts, func() error {
			return git.RunGitIgnoreOutput(projectCheckout, cmd)
		})
		if err != nil {
			errs <- errors.Annotate(err, "could not push branches to remote").Err()
			wg.Done()
			continue
		}
		wg.Done()
	}

	return
}

// CreateRemoteBranches makes the requested branches on the remote Gerrit hosts
// using git checkouts and pushes.
func CreateRemoteBranches(branches []ProjectBranch, dryRun, force bool, workerCount int) error {
	branchChan := make(chan ProjectBranch, len(branches))
	errs := make(chan error, len(branches))

	var wg sync.WaitGroup
	for i := 1; i <= workerCount; i++ {
		go createRemoteBranchesWorker(&wg, branchChan, errs, dryRun, force)
	}

	// Push the local git branches to remote.
	for _, projectBranch := range branches {
		// Don't push the manifest repos because that already happened in RepairManifestRepositories.
		if _, ok := ManifestProjects[projectBranch.Project.Name]; ok {
			continue
		}

		branchChan <- projectBranch
		wg.Add(1)
	}
	close(branchChan)

	wg.Wait()

	select {
	case err := <-errs:
		return err
	default:
	}
	return nil
}

// WhichVersionShouldBump returns which version is incremented by builds on a new branch.
func WhichVersionShouldBump(vinfo mv.VersionInfo) (mv.VersionComponent, error) {
	if vinfo.PatchNumber != 0 {
		return mv.Unspecified, fmt.Errorf("cannot bump version with nonzero patch number")
	}
	if vinfo.BranchBuildNumber != 0 {
		return mv.Patch, nil
	} else {
		return mv.Branch, nil
	}
}

// BranchType determines the type of branch to be created.
func BranchType(release, factory, firmware, stabilize bool, custom string) (string, bool) {
	var branchType string
	branchTypesSelected := 0
	if release {
		branchTypesSelected++
		branchType = "release"
	}
	if factory {
		branchTypesSelected++
		branchType = "factory"
	}
	if firmware {
		branchTypesSelected++
		branchType = "firmware"
	}
	if stabilize {
		branchTypesSelected++
		branchType = "stabilize"
	}
	if custom != "" {
		branchTypesSelected++
		branchType = "custom"
	}
	if branchTypesSelected != 1 {
		return "", false
	}

	return branchType, true
}

// Determine the name for a new branch.
// By convention, standard branch names must end with the stripped version
// string from which they were created, followed by '.B'.
//
// For example:
//	- A branch created from 1.0.0 must end with -1.B
//	- A branch created from 1.2.0 must end with -1.2.B
//
// Release branches have a slightly different naming scheme. They include
//  the milestone from which they were created. Example: release-R12-1.2.B
func NewBranchName(vinfo mv.VersionInfo, custom, descriptor string, release, factory, firmware, stabilize bool) string {
	if custom != "" {
		return custom
	}
	branchType, _ := BranchType(release, factory, firmware, stabilize, custom)
	branchNameParts := []string{branchType}
	if branchType == "release" {
		branchNameParts = append(branchNameParts, fmt.Sprintf("R%d", vinfo.ChromeBranch))
	}
	if descriptor != "" {
		branchNameParts = append(branchNameParts, descriptor)
	}
	branchNameParts = append(branchNameParts, vinfo.StrippedVersionString()+".B")
	return strings.Join(branchNameParts, "-")
}

// CheckIfAlreadyBranched checks if there's already a branch for the desired new
// branch to create on the manifest-internal repo.
func CheckIfAlreadyBranched(vinfo mv.VersionInfo, manifestInternal repo.Project, force bool, branchType string) error {
	// Check that we did not already branch from this version.
	// manifest-internal serves as the sentinel project.
	pattern := regexp.MustCompile(fmt.Sprintf(`.*-%s.B$`, vinfo.StrippedVersionString()))

	// Verify that a major-minor version collision won't occur
	majorMinor := fmt.Sprintf("%v.%v", vinfo.BuildNumber, vinfo.BranchBuildNumber)

	// Fetch remoteUrl
	remoteUrl, err := ProjectFetchUrl(manifestInternal.Path)
	if err != nil {
		err = errors.Annotate(err, "failed to get remote project url").Err()
	}

	// If we give a full URL, don't need to run the command in a git repo.
	remoteBranches, err := git.RemoteBranches("", remoteUrl)
	if err != nil {
		err = errors.Annotate(err, "failed to list remote branches for %s", remoteUrl).Err()
	}

	exists, err := BranchExists(pattern, majorMinor, branchType, remoteBranches)
	if err != nil {
		return err
	}
	if exists {
		if !force {
			return fmt.Errorf("already branched %s. Please rerun with --force if you "+
				"would like to proceed", vinfo.VersionString())
		} else {
			LogErr("Overwriting branch with version %s (--force was set).\n", vinfo.VersionString())
		}
	} else {
		LogErr("No branch exists for version %s. Continuing...\n", vinfo.VersionString())
	}
	return nil
}
