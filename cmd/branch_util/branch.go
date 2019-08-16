// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/chromiumos/infra/go/internal/shared"
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

// branchExists checks that a branch matching the given pattern exists in a particular project.
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

// branchExistsExplicit checks that the given branch exists in the project.
// It is a good bit faster than branchExists.
func branchExistsExplicit(project repo.Project, branch string) (bool, error) {
	remoteUrl, err := projectFetchUrl(project.Path)
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
		output, err := git.RemoteHasBranch("", remoteUrl, branch)
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
		logOut("...checking that %s does not exist in %s.\n",
			projectBranch.branchName,
			projectBranch.project.Name)
		exists, err := branchExistsExplicit(projectBranch.project, projectBranch.branchName)
		if err == nil {
			if exists {
				errs <- fmt.Errorf("Branch %s exists for %s. Please rerun with --force to proceed.",
					projectBranch.branchName, projectBranch.project.Name)
			}
		} else {
			errs <- err
		}
		wg.Done()
	}
}

// assertBranchesDoNotExist checks that branches do not already exist.
func assertBranchesDoNotExist(branches []ProjectBranch) error {
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

// getBranchesByPath returns a map mapping project paths to git branch names.
func getBranchesByPath(branches []ProjectBranch) map[string]string {
	branchesByPath := make(map[string]string)
	for _, branch := range branches {
		branchesByPath[branch.project.Path] = branch.branchName
	}
	return branchesByPath
}

// getOriginRef returns the equivalent of the specified ref for the "origin" remote.
func getOriginRef(ref string) string {
	// If the revision starts with refs/heads/, make it start with
	// refs/remotes/origin instead.
	if git.StripRefsHead(ref) != ref {
		return "refs/remotes/origin/" + git.StripRefsHead(ref)
	}
	// If the revision is a SHA, let it be.
	return ref
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
		opts := &checkoutOptions{
			depth: 1,
			ref:   manifestProject.Revision,
		}
		manifestCheckout, err := getProjectCheckout(manifestProject.Path, opts)
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
		opts := &checkoutOptions{
			depth: 1,
			ref:   projectBranch.project.Revision,
		}
		projectCheckout, err := getProjectCheckout(projectBranch.project.Path, opts)
		defer os.RemoveAll(projectCheckout)
		if err != nil {
			errs <- errors.Annotate(err, "could not checkout %s:%s",
				projectBranch.project.Path, projectBranch.branchName).Err()
			wg.Done()
			continue
		}

		branchName := git.NormalizeRef(projectBranch.branchName)
		refspec := fmt.Sprintf("%s:%s", getOriginRef(projectBranch.project.Revision), branchName)

		cmd := []string{"push", "origin", refspec}
		if dryRun {
			cmd = append(cmd, "--dry-run")
		}
		if force {
			cmd = append(cmd, "--force")
		}
		logOut("Pushing ref %s for project %s.\n", branchName, projectBranch.project.Path)

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

func createRemoteBranches(branches []ProjectBranch, dryRun, force bool) error {
	branchChan := make(chan ProjectBranch, len(branches))
	errs := make(chan error, len(branches))

	var wg sync.WaitGroup
	for i := 1; i <= workerCount; i++ {
		go createRemoteBranchesWorker(&wg, branchChan, errs, dryRun, force)
	}

	// Push the local git branches to remote.
	for _, projectBranch := range branches {
		// Don't push the manifest repos because that already happened in repairManifestRepositories.
		if _, ok := ManifestProjects[projectBranch.project.Name]; ok {
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

// whichVersionShouldBump returns which version is incremented by builds on a new branch.
func whichVersionShouldBump(vinfo mv.VersionInfo) (mv.VersionComponent, error) {
	if vinfo.PatchNumber != 0 {
		return mv.Unspecified, fmt.Errorf("cannot bump version with nonzero patch number")
	}
	if vinfo.BranchBuildNumber != 0 {
		return mv.Patch, nil
	} else {
		return mv.Branch, nil
	}
}
