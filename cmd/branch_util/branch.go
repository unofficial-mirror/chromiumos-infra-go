// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/manifest_repo"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

var (
	MANIFEST_PROJECTS = []string{"chromiumos/manifest", "chromeos/manifest-internal"}
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

// assertBranchesDoNotExist checks that branches do not already exist.
func assertBranchesDoNotExist(branches []ProjectBranch) error {
	for _, projectBranch := range branches {
		pattern := regexp.MustCompile(projectBranch.branchName)
		exists, err := checkout.BranchExists(projectBranch.project, pattern)
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
	for _, projectName := range MANIFEST_PROJECTS {
		manifestProject, err := workingManifest.GetUniqueProject(projectName)
		if err != nil {
			return err
		}
		manifestRepo := manifest_repo.ManifestRepo{
			Checkout: checkout,
			Project:  manifestProject,
		}
		if err := manifestRepo.RepairManifestsOnDisk(getBranchesByPath(branches)); err != nil {
			return errors.Annotate(err, "failed to repair manifest project %s", projectName).Err()
		}
		if _, err := git.RunGit(checkout.AbsoluteProjectPath(manifestProject),
			[]string{"commit", "-a", "-m", "commit repaired manifests"}); err != nil {
			return errors.Annotate(err, "error committing repaired manifests").Err()
		}
	}
	// Push the local git branches to remote.
	for _, projectBranch := range branches {
		branchName := git.NormalizeRef(projectBranch.branchName)

		// The refspec should look like 'HEAD:refs/heads/branchName'.
		refspec := fmt.Sprintf("HEAD:%s", branchName)
		remote := workingManifest.GetRemoteByName(projectBranch.project.RemoteName).GitName()

		cmd := []string{"push", remote, refspec}
		if dryRun {
			cmd = append(cmd, "--dry-run")
		}
		if force {
			cmd = append(cmd, "--force")
		}

		if _, err := checkout.RunGit(projectBranch.project, cmd); err != nil {
			return errors.Annotate(err, "could not push branches to remote").Err()
		}
	}

	return nil
}

// whichVersionShouldBump returns which version is incremented by builds on a new branch.
func whichVersionShouldBump() (repo.VersionComponent, error) {
	vinfo, err := checkout.ReadVersion()
	if err != nil {
		return repo.Unspecified, err
	}
	if vinfo.PatchNumber != 0 {
		return repo.Unspecified, fmt.Errorf("cannot bump version with nonzero patch number")
	}
	if vinfo.BranchBuildNumber != 0 {
		return repo.Patch, nil
	} else {
		return repo.Branch, nil
	}
}
