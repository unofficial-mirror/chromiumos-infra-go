// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

var (
	ManifestProjects = map[string]bool{
		"chromiumos/manifest":        true,
		"chromeos/manifest-internal": true,
	}
)

type ProjectBranch struct {
	project    repo.Project
	branchName string
}

// canBranchProject retuns true if the project can be branched.
func canBranchProject(manifest repo.Manifest, project repo.Project) bool {
	return manifest.ProjectBranchMode(project) == repo.Create
}

// projectBranchName determines the git branch name for the project.
func projectBranchName(branch string, project repo.Project, original string) string {
	// If the project has only one checkout, then the base branch name is fine.
	var checkouts []string
	for _, proj := range workingManifest.Projects {
		if proj.Name == project.Name {
			checkouts = append(checkouts, proj.Name)
		}
	}

	if len(checkouts) == 1 {
		return branch
	}

	// Otherwise, the project name needs a suffix. We append its upstream or
	// revision to distinguish it from other checkouts.
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
	return branch + suffix
}

// projectBranches returns a list of ProjectBranch structs:
// one for each branchable project.
// The original parameter is the CrOS branch from which the current checkout stems.
func projectBranches(branch, original string) []ProjectBranch {
	var projectBranches []ProjectBranch
	for _, project := range workingManifest.Projects {
		if canBranchProject(workingManifest, project) {
			projectBranches = append(projectBranches,
				ProjectBranch{
					project:    project,
					branchName: projectBranchName(branch, project, original),
				})
		}
	}
	return projectBranches
}

// branchExists checks that a branch exists in a particular project.
func branchExists(project repo.Project, branchPattern *regexp.Regexp) (bool, error) {
	remoteUrl, err := projectFetchUrl(project.Path)
	if err != nil {
		return false, errors.Annotate(err, "failed to get remote project url").Err()
	}

	// If we give a full URL, don't need to run the command in a git repo.
	remoteBranches, err := git.RemoteBranches("", remoteUrl)
	if err != nil {
		return false, errors.Annotate(err, "failed to list remote branches for %s", remoteUrl).Err()
	}

	for _, branch := range remoteBranches {
		if branchPattern.Match([]byte(branch)) {
			return true, nil
		}
	}

	return false, nil
}

// assertBranchesDoNotExist checks that branches do not already exist.
func assertBranchesDoNotExist(branches []ProjectBranch) error {
	for _, projectBranch := range branches {
		pattern := regexp.MustCompile(projectBranch.branchName)
		exists, err := branchExists(projectBranch.project, pattern)
		if err != nil {
			return errors.Annotate(err, "Error checking existence of branch %s in %s.",
				projectBranch.branchName, projectBranch.project.Name).Err()
		}
		if exists {
			return fmt.Errorf("Branch %s exists for %s. Please rerun with --force to proceed.",
				projectBranch.branchName, projectBranch.project.Name)
		}
	}
	return nil
}

// getBranchesByPath returns a map mapping project paths to git branch names.
func getBranchesByPath(branches []ProjectBranch) map[string]string {
	branchesByPath := make(map[string]string)
	for _, branch := range branches {
		branchesByPath[branch.project.Path] = branch.branchName
	}
	return branchesByPath
}

// repairManifestRepositories repairs all manifests in all manifest repositories
// on the current branch and commits the changes. It then pushes the state of
// the local git branches to remote.
func repairManifestRepositories(branches []ProjectBranch, dryRun, force bool) error {
	manifestBranchNames := make(map[string]string)

	// Find names of manifest project branches so that we can push changes.
	for _, projectBranch := range branches {
		if _, ok := ManifestProjects[projectBranch.project.Name]; ok {
			manifestBranchNames[projectBranch.project.Name] = projectBranch.branchName
		}
	}

	for projectName := range ManifestProjects {
		manifestProject, err := workingManifest.GetUniqueProject(projectName)
		if err != nil {
			return err
		}
		manifestCheckout, err := getProjectCheckout(manifestProject.Path)
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
		if _, err := git.RunGit(manifestCheckout,
			[]string{"commit", "-a", "-m", "commit repaired manifests"}); err != nil {
			return errors.Annotate(err, "error committing repaired manifests").Err()
		}
		refspec := fmt.Sprintf("HEAD:%s", manifestBranchNames[projectName])

		// TODO(@jackneus): Replace with git.Push call after git package is cleaned up.
		cmd := []string{"push", "origin", refspec}
		if dryRun {
			cmd = append(cmd, "--dry-run")
		}
		if force {
			cmd = append(cmd, "--force")
		}
		if _, err := git.RunGit(manifestCheckout, cmd); err != nil {
			return errors.Annotate(err, "could not push branches to remote").Err()
		}
	}
	return nil
}

func createRemoteBranches(branches []ProjectBranch, dryRun, force bool) error {
	// Push the local git branches to remote.
	for _, projectBranch := range branches {
		// Don't push the manifest repos because that already happened in repairManifestRepositories.
		if _, ok := ManifestProjects[projectBranch.project.Name]; ok {
			continue
		}
		projectCheckout, err := getProjectCheckout(projectBranch.project.Path)
		defer os.RemoveAll(projectCheckout)
		if err != nil {
			return errors.Annotate(err, "could not checkout %s:%s",
				projectBranch.project.Path, projectBranch.branchName).Err()
		}

		branchName := git.NormalizeRef(projectBranch.branchName)

		// If the revision is a SHA, let it be. Otherwise we need to strip refs/...
		revision := projectBranch.project.Revision
		// i.e. if revision starts with refs/...
		if git.StripRefs(revision) != revision {
			revision = "refs/remotes/origin/" + git.StripRefs(revision)
		}
		refspec := fmt.Sprintf("%s:%s", revision, branchName)

		cmd := []string{"push", "origin", refspec}
		if dryRun {
			cmd = append(cmd, "--dry-run")
		}
		if force {
			cmd = append(cmd, "--force")
		}
		if _, err := git.RunGit(projectCheckout, cmd); err != nil {
			return errors.Annotate(err, "could not push branches to remote").Err()
		}
	}

	return nil
}

// whichVersionShouldBump returns which version is incremented by builds on a new branch.
func whichVersionShouldBump(vinfo repo.VersionInfo) (repo.VersionComponent, error) {
	if vinfo.PatchNumber != 0 {
		return repo.Unspecified, fmt.Errorf("cannot bump version with nonzero patch number")
	}
	if vinfo.BranchBuildNumber != 0 {
		return repo.Patch, nil
	} else {
		return repo.Branch, nil
	}
}
