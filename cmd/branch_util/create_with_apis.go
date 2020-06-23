// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"context"
	"fmt"
	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/branch"
	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

const (
	chromeOsVersionProjectPath = "src/third_party/chromiumos-overlay"
	branchCreatorGroup         = "mdb/chromeos-branch-creators"
)

func getCmdCreateBranchV2(opts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create-v2 <options>",
		ShortDesc: "Create a branch.",
		LongDesc:  "Create a branch using the newer Gerrit API-based branching approach.",
		CommandRun: func() subcommands.CommandRun {
			c := &createBranchV2{}
			c.Init(opts)
			// Arguments for determining branch name.
			c.Flags.StringVar(&c.descriptor, "descriptor", "",
				"Optional descriptor for this branch. Typically, this is a build "+
					"target or a device, depending on the nature of the branch. Used "+
					"to generate the branch name. Cannot be used with --custom.")
			c.Flags.StringVar(&c.buildSpecManifest, "buildspec-manifest", "",
				"Path to manifest within manifest-versions repo, relative to "+
					"https://chrome-internal.googlesource.com/chromeos/manifest-versions/+/master/buildspecs/ "+
					"e.g. 85/13277.0.0.xml")
			c.Flags.BoolVar(&c.release, "release", false,
				"The new branch is a release branch. "+
					"Named as 'release-<descriptor>-R<Milestone>-<Major Version>.B'.")
			c.Flags.BoolVar(&c.factory, "factory", false,
				"The new branch is a factory branch. "+
					"Named as 'factory-<Descriptor>-<Major Version>.B'.")
			c.Flags.BoolVar(&c.firmware, "firmware", false,
				"The new branch is a firmware branch. "+
					"Named as 'firmware-<Descriptor>-<Major Version>.B'.")
			c.Flags.BoolVar(&c.stabilize, "stabilize", false,
				"The new branch is a minibranch. "+
					"Named as 'stabilize-<Descriptor>-<Major Version>.B'.")
			c.Flags.StringVar(&c.custom, "custom", "",
				"Use a custom branch type with an explicit name. "+
					"WARNING: custom names are dangerous. This tool greps branch "+
					"names to determine which versions have already been branched. "+
					"Version validation is not possible when the naming convention "+
					"is broken. Use this at your own risk.")
			return c
		},
	}
}

type createBranchV2 struct {
	CommonFlags
	yes               bool
	descriptor        string
	version           string
	buildSpecManifest string
	release           bool
	factory           bool
	firmware          bool
	stabilize         bool
	custom            string
}

func (c *createBranchV2) getBranchType() (string, bool) {
	var branch_type string
	branch_types_selected := 0
	if c.release {
		branch_types_selected++
		branch_type = "release"
	}
	if c.factory {
		branch_types_selected++
		branch_type = "factory"
	}
	if c.firmware {
		branch_types_selected++
		branch_type = "firmware"
	}
	if c.stabilize {
		branch_types_selected++
		branch_type = "stabilize"
	}
	if c.custom != "" {
		branch_types_selected++
		branch_type = "custom"
	}
	if branch_types_selected != 1 {
		return "", false
	}

	return branch_type, true
}

func (c *createBranchV2) validate(args []string) (bool, string) {
	if c.buildSpecManifest == "" {
		return false, "must set --buildspec-manifest"
	}
	_, ok := c.getBranchType()
	if !ok {
		return false, "must select exactly one branch type " +
			"(--release, --factory, --firmware, --stabilize, --custom)."
	}
	if c.descriptor != "" && c.custom != "" {
		return false, "--descriptor cannot be used with --custom."
	}
	if c.version != "" && c.version[len(c.version)-1] != '0' {
		return false, "cannot branch version from nonzero patch number."
	}
	return true, ""
}

// Getters so that functions using the branchCommand interface
// can access CommonFlags in the underlying struct.
func (c *createBranchV2) getRoot() string {
	return c.Root
}

func (c *createBranchV2) getManifestUrl() string {
	return c.ManifestUrl
}

// Determine the name for a branch.
// By convention, standard branch names must end with the stripped version
// string from which they were created, followed by '.B'.
//
// For example:
//	- A branch created from 1.0.0 must end with -1.B
//	- A branch created from 1.2.0 must end with -1.2.B
//
// Release branches have a slightly different naming scheme. They include
//  the milestone from which they were created. Example: release-R12-1.2.B
func (c *createBranchV2) newBranchName(vinfo mv.VersionInfo) string {
	if c.custom != "" {
		return c.custom
	}
	branchType, _ := c.getBranchType()
	branchNameParts := []string{branchType}
	if branchType == "release" {
		branchNameParts = append(branchNameParts, fmt.Sprintf("R%d", vinfo.ChromeBranch))
	}
	if c.descriptor != "" {
		branchNameParts = append(branchNameParts, c.descriptor)
	}
	branchNameParts = append(branchNameParts, vinfo.StrippedVersionString()+".B")
	return strings.Join(branchNameParts, "-")
}

func (c *createBranchV2) bumpVersion(
	component mv.VersionComponent,
	branch, commitMsg string,
	dryRun bool) error {
	// Branch won't exist if running tool with --dry-run.
	if dryRun {
		return nil
	}
	if component == mv.Unspecified {
		return fmt.Errorf("component was unspecified")
	}

	// Get checkout of versionProjectPath, which has chromeos_version.sh.
	opts := &checkoutOptions{
		depth: 1,
		ref:   branch,
	}
	versionProjectCheckout, err := getProjectCheckout(chromeOsVersionProjectPath, opts)
	defer os.RemoveAll(versionProjectCheckout)
	if err != nil {
		return errors.Annotate(err, "bumpVersion: local checkout of version project failed").Err()
	}

	version, err := mv.GetVersionInfoFromRepo(versionProjectCheckout)
	if err != nil {
		return errors.Annotate(err, "failed to read version file").Err()
	}

	version.IncrementVersion(component)
	// We are cloning from a remote, so the remote name will be origin.
	remoteRef := git.RemoteRef{
		Remote: "origin",
		Ref:    git.NormalizeRef(branch),
	}

	if err := version.UpdateVersionFile(); err != nil {
		return errors.Annotate(err, "failed to update version file").Err()
	}

	_, err = git.CommitAll(versionProjectCheckout, commitMsg)
	gitOpts := git.GitOpts{
		DryRun: dryRun,
		Force:  false,
	}
	errs := []error{
		err,
		git.PushRef(versionProjectCheckout, "HEAD", remoteRef, gitOpts),
	}
	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to push version changes to remote").Err()
		}
	}
	return nil
}

func (c *createBranchV2) Run(a subcommands.Application, args []string,
	env subcommands.Env) int {
	// Common setup (argument validation, repo init, etc.)
	ret := Run(c, a, args, env)
	if ret != 0 {
		return ret
	}
	ctx := context.Background()
	authOpts, err := c.authFlags.Options()
	if err != nil {
		logErr(errors.Annotate(err, "failed to configure auth").Err().Error())
		return 1
	}
	authedClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		logErr(errors.Annotate(err, "Please run `./branch_util auth-login` and sign in with your @google.com account").Err().Error())
		return 1
	}

	if c.Push {
		inGroup, err := branch.CheckSelfGroupMembership(authedClient, "https://chromium-review.googlesource.com", branchCreatorGroup)
		if err != nil {
			logErr(errors.Annotate(err, "failed to confirm that the running user is in %v", branchCreatorGroup).Err().Error())
			return 1
		}
		if !inGroup {
			logErr("you appear not to be in %v, and so you won't be able to create a branch", branchCreatorGroup)
			return 1
		}
	}

	file, err := gerrit.DownloadFileFromGitiles(authedClient, ctx, "chrome-internal.googlesource.com",
		"chromeos/manifest-versions", "master", "buildspecs/"+c.buildSpecManifest)
	if err != nil {
		logErr(errors.Annotate(err, "failed to fetch buildspec %v", c.buildSpecManifest).Err().Error())
		return 1
	}
	logErr("Got %v from Gitiles", c.buildSpecManifest)
	wm, err := ioutil.TempFile("", "working-manifest.xml")
	if err != nil {
		logErr("%s\n", err.Error())
		return 1
	}
	_, err = wm.WriteString(file)
	if err != nil {
		logErr("%s\n", err.Error())
		return 1
	}
	workingManifest, err = repo.LoadManifestFromFile(wm.Name())
	if err != nil {
		err = errors.Annotate(err, "failed to load manifests").Err()
		logErr("%s\n", err.Error())
		return 1
	}
	logErr("Fetched working manifest.\n")

	// Use manifest-internal as a sentinel repository to get the appropriate branch name.
	// We know that manifest-internal is a single-checkout so its revision should be
	// master or the name of the Chrome OS branch.
	manifestInternal, err := workingManifest.GetUniqueProject("chromeos/manifest-internal")
	if err != nil {
		logErr(errors.Annotate(err, "Could not get chromeos/manifest-internal project.").Err().Error())
		return 1
	}
	sourceRevision := manifestInternal.Revision
	sourceUpstream := git.StripRefs(manifestInternal.Upstream)
	logErr("Using sourceRevision %s for manifestInternal", sourceRevision)
	logErr("Using sourceUpstream %s for manifestInternal", sourceUpstream)

	// Validate the version.
	// Double check that the checkout has a zero patch number. Otherwise we cannot branch from it.
	versionProject, err := workingManifest.GetProjectByPath(versionProjectPath)
	if err != nil {
		err = errors.Annotate(err, "could not get project %s from manifest", versionProjectPath).Err()
		logErr("%s\n", err)
		return 1
	}

	// Fetch chromeos_version.sh from the source branch
	versionFile, err := gerrit.DownloadFileFromGitiles(authedClient, ctx,
		"chromium.googlesource.com", versionProject.Name, versionProject.Revision, mv.VersionFileProjectPath)
	if err != nil {
		logErr(errors.Annotate(err, "failed to fetch versionFile").Err().Error())
		return 1
	}

	vinfo, err := mv.ParseVersionInfo([]byte(versionFile))
	if err != nil {
		logErr(errors.Annotate(err, "error reading version").Err().Error())
		return 1
	}

	if vinfo.PatchNumber != 0 {
		logErr("Cannot branch version with nonzero patch number (version %s).",
			vinfo.VersionString())
		return 1
	}
	logErr("Version found: %s.\n", vinfo.VersionString())

	logErr("Have manifest = %v", manifestInternal)

	// Check that we did not already branch from this version.
	// manifest-internal serves as the sentinel project.
	pattern := regexp.MustCompile(fmt.Sprintf(`.*-%s.B$`, vinfo.StrippedVersionString()))
	exists, err := branchExists(manifestInternal, pattern)
	if err != nil {
		logErr(err.Error())
		return 1
	}
	if exists {
		if !c.Force {
			logErr("Already branched %s. Please rerun with --force if you  would like to proceed.",
				vinfo.VersionString())
			return 1
		} else {
			logOut("Overwriting branch with version %s (--force was set).\n", vinfo.VersionString())
		}
	} else {
		logOut("No branch exists for version %s. Continuing...\n", vinfo.VersionString())
	}

	// Generate branch name.
	branchName := c.newBranchName(vinfo)
	// Create branch.
	componentToBump, err := whichVersionShouldBump(vinfo)
	if err != nil {
		logErr(err.Error())
		return 1
	}

	// Generate git branch names.
	branches := projectBranches(branchName, git.StripRefs(sourceRevision))
	logOut("Creating branch: %s\n", branchName)

	projectBranches, err := gerritProjectBranches(branches)
	if err != nil {
		logErr(err.Error())
		return 1
	}

	// If not --force, validate branch names to ensure that they do not already exist.
	if !c.Force {
		err = branch.AssertBranchesDoNotExist(authedClient, projectBranches)
		if err != nil {
			logErr(err.Error())
			return 1
		}
	}
	logOut("Done validating project branches.\n")

	// Repair manifest repositories.
	if err = repairManifestRepositories(branches, !c.Push, c.Force); err != nil {
		logErr(err.Error())
		return 1
	}

	// Create git branches for new branch. Exclude the ManifestProjects, which we just updated.
	if err = branch.CreateRemoteBranches(authedClient, getNonManifestBranches(projectBranches), !c.Push, c.Force); err != nil {
		logErr(err.Error())
		return 1
	}

	// Bump version.
	commitMsg := fmt.Sprintf("Bump %s number after creating branch %s", componentToBump, branchName)
	logErr(commitMsg)
	if err = c.bumpVersion(componentToBump, branchName, commitMsg, !c.Push); err != nil {
		logErr(err.Error())
		return 1
	}

	if c.release {
		// Bump milestone after creating release branch.
		commitMsg = fmt.Sprintf("Bump milestone after creating release branch %s", branchName)
		logErr(commitMsg)
		if err = c.bumpVersion(mv.ChromeBranch, sourceUpstream, commitMsg, !c.Push); err != nil {
			logErr(err.Error())
			return 1
		}
		// Also need to bump the build number, otherwise two release will have conflicting versions.
		// See crbug.com/213075.
		commitMsg = fmt.Sprintf("Bump build number after creating release branch %s", branchName)
		logErr(commitMsg)
		if err = c.bumpVersion(mv.Build, sourceUpstream, commitMsg, !c.Push); err != nil {
			logErr(err.Error())
			return 1
		}
	} else {
		// For non-release branches, we also have to bump some component of the source branch.
		// This is so that subsequent branches created from the source branch do not conflict
		// with the branch we just created.
		// Example:
		// Say we just branched off of our source branch (version 1.2.0). The newly-created branch
		// has version 1.2.1. If later on somebody tries to branch off of the source branch again,
		// a second branch will be created with version 1.2.0. This is problematic.
		// To avoid this, we bump the source branch. So in this case, we would bump 1.2.0 --> 1.3.0.
		// See crbug.com/965164 for context.
		var sourceComponentToBump mv.VersionComponent
		if componentToBump == mv.Patch {
			sourceComponentToBump = mv.Branch
		} else {
			sourceComponentToBump = mv.Build
		}
		commitMsg = fmt.Sprintf("Bump %s number for source branch %s after creating branch %s",
			sourceComponentToBump, sourceUpstream, branchName)
		logErr(commitMsg)
		err = c.bumpVersion(sourceComponentToBump, sourceUpstream, commitMsg, !c.Push)
		if err != nil {
			logErr(err.Error())
			return 1
		}
	}

	if !c.Push {
		logErr("Dry run (no --push): completed successfully")
	}
	return 0
}
