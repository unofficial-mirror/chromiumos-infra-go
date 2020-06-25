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
	"regexp"
	"strings"
)

const (
	branchCreatorGroup = "mdb/chromeos-branch-creators"
)

func getCmdCreateBranchV2(opts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create-v2 <options>",
		ShortDesc: "Create a branch.",
		LongDesc:  "Create a branch using the newer Gerrit API-based branching approach.",
		CommandRun: func() subcommands.CommandRun {
			c := &createBranchV2{}
			c.InitFlags(opts)
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
			c.Flags.Float64Var(&c.gerritReadQps, "gerrit-read-qps", 40.0,
				"Maximum QPS to use for Gerrit API read operations.")
			c.Flags.Float64Var(&c.gerritWriteQps, "gerrit-write-qps", 5.0,
				"Maximum QPS to use for Gerrit API write operations.")
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
	gerritReadQps     float64
	gerritWriteQps    float64
}

func (c *createBranchV2) getBranchType() (string, bool) {
	var branchType string
	branchTypesSelected := 0
	if c.release {
		branchTypesSelected++
		branchType = "release"
	}
	if c.factory {
		branchTypesSelected++
		branchType = "factory"
	}
	if c.firmware {
		branchTypesSelected++
		branchType = "firmware"
	}
	if c.stabilize {
		branchTypesSelected++
		branchType = "stabilize"
	}
	if c.custom != "" {
		branchTypesSelected++
		branchType = "custom"
	}
	if branchTypesSelected != 1 {
		return "", false
	}

	return branchType, true
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
		branch.LogErr(errors.Annotate(err, "failed to configure auth").Err().Error())
		return 1
	}
	authedClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		branch.LogErr(errors.Annotate(err, "Please run `./branch_util auth-login` and sign in with your @google.com account").Err().Error())
		return 1
	}

	if c.Push {
		inGroup, err := branch.CheckSelfGroupMembership(authedClient, "https://chromium-review.googlesource.com", branchCreatorGroup)
		if err != nil {
			branch.LogErr(errors.Annotate(err, "failed to confirm that the running user is in %v", branchCreatorGroup).Err().Error())
			return 1
		}
		if !inGroup {
			branch.LogErr("you appear not to be in %v, and so you won't be able to create a branch", branchCreatorGroup)
			return 1
		}
	}

	file, err := gerrit.DownloadFileFromGitiles(authedClient, ctx, "chrome-internal.googlesource.com",
		"chromeos/manifest-versions", "master", "buildspecs/"+c.buildSpecManifest)
	if err != nil {
		branch.LogErr(errors.Annotate(err, "failed to fetch buildspec %v", c.buildSpecManifest).Err().Error())
		return 1
	}
	branch.LogErr("Got %v from Gitiles", c.buildSpecManifest)
	wm, err := ioutil.TempFile("", "working-manifest.xml")
	if err != nil {
		branch.LogErr("%s\n", err.Error())
		return 1
	}
	_, err = wm.WriteString(file)
	if err != nil {
		branch.LogErr("%s\n", err.Error())
		return 1
	}
	branch.WorkingManifest, err = repo.LoadManifestFromFile(wm.Name())
	if err != nil {
		err = errors.Annotate(err, "failed to load manifests").Err()
		branch.LogErr("%s\n", err.Error())
		return 1
	}
	branch.LogErr("Fetched working manifest.\n")

	// Use manifest-internal as a sentinel repository to get the appropriate branch name.
	// We know that manifest-internal is a single-checkout so its revision should be
	// master or the name of the Chrome OS branch.
	manifestInternal, err := branch.WorkingManifest.GetUniqueProject("chromeos/manifest-internal")
	if err != nil {
		branch.LogErr(errors.Annotate(err, "Could not get chromeos/manifest-internal project.").Err().Error())
		return 1
	}
	sourceRevision := manifestInternal.Revision
	sourceUpstream := git.StripRefs(manifestInternal.Upstream)
	branch.LogErr("Using sourceRevision %s for manifestInternal", sourceRevision)
	branch.LogErr("Using sourceUpstream %s for manifestInternal", sourceUpstream)

	// Validate the version.
	// Double check that the checkout has a zero patch number. Otherwise we cannot branch from it.
	versionProject, err := branch.WorkingManifest.GetProjectByPath(branch.VersionFileProjectPath)
	if err != nil {
		err = errors.Annotate(err, "could not get project %s from manifest", branch.VersionFileProjectPath).Err()
		branch.LogErr("%s\n", err)
		return 1
	}

	// Fetch chromeos_version.sh from the source branch
	versionFile, err := gerrit.DownloadFileFromGitiles(authedClient, ctx,
		"chromium.googlesource.com", versionProject.Name, versionProject.Revision, mv.VersionFileProjectPath)
	if err != nil {
		branch.LogErr(errors.Annotate(err, "failed to fetch versionFile").Err().Error())
		return 1
	}

	vinfo, err := mv.ParseVersionInfo([]byte(versionFile))
	if err != nil {
		branch.LogErr(errors.Annotate(err, "error reading version").Err().Error())
		return 1
	}

	if vinfo.PatchNumber != 0 {
		branch.LogErr("Cannot branch version with nonzero patch number (version %s).",
			vinfo.VersionString())
		return 1
	}
	branch.LogErr("Version found: %s.\n", vinfo.VersionString())

	branch.LogErr("Have manifest = %v", manifestInternal)

	// Check that we did not already branch from this version.
	// manifest-internal serves as the sentinel project.
	pattern := regexp.MustCompile(fmt.Sprintf(`.*-%s.B$`, vinfo.StrippedVersionString()))
	exists, err := branch.BranchExists(manifestInternal, pattern)
	if err != nil {
		branch.LogErr(err.Error())
		return 1
	}
	if exists {
		if !c.Force {
			branch.LogErr("Already branched %s. Please rerun with --force if you  would like to proceed.",
				vinfo.VersionString())
			return 1
		} else {
			branch.LogOut("Overwriting branch with version %s (--force was set).\n", vinfo.VersionString())
		}
	} else {
		branch.LogOut("No branch exists for version %s. Continuing...\n", vinfo.VersionString())
	}

	// Generate branch name.
	branchName := c.newBranchName(vinfo)
	// Create branch.
	componentToBump, err := branch.WhichVersionShouldBump(vinfo)
	if err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Generate git branch names.
	branches := branch.ProjectBranches(branchName, git.StripRefs(sourceRevision))
	branch.LogOut("Creating branch: %s\n", branchName)

	projectBranches, err := branch.GerritProjectBranches(branches)
	if err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// If not --force, validate branch names to ensure that they do not already exist.
	if !c.Force {
		err = branch.AssertBranchesDoNotExistApi(authedClient, projectBranches, c.gerritReadQps)
		if err != nil {
			branch.LogErr(err.Error())
			return 1
		}
	}
	branch.LogOut("Done validating project branches.\n")

	// Repair manifest repositories.
	if err = branch.RepairManifestRepositories(branches, !c.Push, c.Force); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Create git branches for new branch. Exclude the ManifestProjects, which we just updated.
	if err = branch.CreateRemoteBranchesApi(authedClient, branch.GetNonManifestBranches(projectBranches), !c.Push, c.Force, c.gerritWriteQps); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Bump version.
	if err = branch.BumpForCreate(componentToBump, c.release, c.Push, branchName, sourceUpstream); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	if !c.Push {
		branch.LogErr("Dry run (no --push): completed successfully")
	}
	return 0
}
