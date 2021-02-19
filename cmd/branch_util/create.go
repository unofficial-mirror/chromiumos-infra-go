// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/branch"
	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
)

const (
	branchCreatorGroup = "mdb/chromeos-branch-creators"
)

func getCmdCreateBranch(opts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create <options>",
		ShortDesc: "Create a branch.",
		LongDesc:  "Create a branch using the newer Gerrit API-based branching approach.",
		CommandRun: func() subcommands.CommandRun {
			c := &createBranch{}
			c.InitFlags(opts)
			// Arguments for determining branch name.
			c.Flags.StringVar(&c.file, "file", "",
				"File path to manifest file. Can be either absolute or relative to branch_util binary.")
			c.Flags.StringVar(&c.descriptor, "descriptor", "",
				"Optional descriptor for this branch. Typically, this is a build "+
					"target or a device, depending on the nature of the branch. Used "+
					"to generate the branch name. Cannot be used with --custom.")
			c.Flags.StringVar(&c.buildSpecManifest, "buildspec-manifest", "",
				"Path to manifest within manifest-versions repo, relative to "+
					"https://chrome-internal.googlesource.com/chromeos/manifest-versions/+/HEAD/buildspecs/ "+
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
			c.Flags.Float64Var(&c.gerritWriteQps, "gerrit-write-qps", 1.0,
				"Maximum QPS to use for Gerrit API write operations.")
			return c
		},
	}
}

type createBranch struct {
	CommonFlags
	yes               bool
	descriptor        string
	buildSpecManifest string
	release           bool
	factory           bool
	firmware          bool
	stabilize         bool
	custom            string
	gerritWriteQps    float64
	file              string
}

func (c *createBranch) validate(args []string) (bool, string) {
	if c.buildSpecManifest == "" && c.file == "" {
		return false, "must set --buildspec-manifest or --file"
	}
	if c.buildSpecManifest != "" && c.file != "" {
		return false, "--buildspec-manifest and --file cannot be used together"
	}
	_, ok := branch.BranchType(c.release, c.factory, c.firmware, c.stabilize, c.custom)
	if !ok {
		return false, "must select exactly one branch type " +
			"(--release, --factory, --firmware, --stabilize, --custom)."
	}
	if c.descriptor != "" && c.custom != "" {
		return false, "--descriptor cannot be used with --custom."
	}
	return true, ""
}

// Getters so that functions using the branchCommand interface
// can access CommonFlags in the underlying struct.
func (c *createBranch) getRoot() string {
	return c.Root
}

func (c *createBranch) getManifestUrl() string {
	return c.ManifestUrl
}

func (c *createBranch) Run(a subcommands.Application, args []string,
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
		branch.LogErr(errors.Annotate(err, "Please run `%s auth-login` and sign in with your @google.com account", os.Args[0]).Err().Error())
		return 1
	}

	// Check if the user is in mdb/chromeos-branch-creators, unless SkipGroupCheck is set.
	// This is not to say that an unauthorized user can simply call the tool with --skip-group-check;
	// ACLs will still be enforced. Skipping this check is necessary for bot invocations,
	// as service accounts cannot be added to MDB groups.
	if c.Push && !c.SkipGroupCheck {
		inGroup, err := branch.CheckSelfGroupMembership(authedClient, "https://chromium-review.googlesource.com", branchCreatorGroup)
		if err != nil {
			branch.LogErr(errors.Annotate(err, "failed to confirm that the running user is in %v", branchCreatorGroup).Err().Error())
			return 1
		}
		if !inGroup {
			branch.LogErr("you appear not to be in %v, and so you won't be able to create a branch.\n"+
				"See http://go/cros-branch#access for instructions for gaining access.", branchCreatorGroup)
			return 1
		}
	}

	if c.file != "" {
		// Branch from file.
		file, err := repo.LoadManifestFromFileWithIncludes(c.file)
		if err != nil {
			branch.LogErr(errors.Annotate(err, "Error: Failed to load manifest from file ").Err().Error())
			return 1
		}
		branch.LogOut("Got manifest from filepath %v", c.file)
		branch.WorkingManifest = *file
	} else {
		file, err := gerrit.DownloadFileFromGitiles(authedClient, ctx, "chrome-internal.googlesource.com",
			"chromeos/manifest-versions", "HEAD", "buildspecs/"+c.buildSpecManifest)
		if err != nil {
			branch.LogErr(errors.Annotate(err, "failed to fetch buildspec %v", c.buildSpecManifest).Err().Error())
			return 1
		}
		branch.LogOut("Got %v from Gitiles", c.buildSpecManifest)
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
		branch.LogOut("Fetched working manifest.\n")
	}

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

	// This string replacement is needed since chromiumos-overlay's master branch
	// has been renamed to main. This replacement swaps the name so the correct
	// upstream is used. crbug.com/1163216 for reference.
	if sourceUpstream == "master" {
		sourceUpstream = "main"
	}

	branch.LogOut("Using sourceRevision %s for manifestInternal", sourceRevision)
	branch.LogOut("Using sourceUpstream %s for manifestInternal", sourceUpstream)

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
	branch.LogOut("Version found: %s.\n", vinfo.VersionString())

	branch.LogOut("Have manifest = %v", manifestInternal)

	branchType := ""

	switch {
	case c.release:
		branchType = "release"
	case c.factory:
		branchType = "factory"
	case c.firmware:
		branchType = "firmware"
	case c.stabilize:
		branchType = "stabilize"
	default:
		branchType = "custom"

	}

	if err = branch.CheckIfAlreadyBranched(vinfo, manifestInternal, c.Force, branchType); err != nil {
		branch.LogErr("%v", err)
		return 1
	}

	branchName := branch.NewBranchName(vinfo, c.custom, c.descriptor, c.release, c.factory, c.firmware, c.stabilize)
	componentToBump, err := branch.WhichVersionShouldBump(vinfo)
	if err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Generate git branch names.
	branches := branch.ProjectBranches(branchName, git.StripRefs(sourceRevision))
	// Do not change the format of this string, it is parsed by the brancher recipe.
	branch.LogOut("Creating branch: %s\n", branchName)

	projectBranches, err := branch.GerritProjectBranches(branches)
	if err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Repair manifest repositories.
	if err = branch.RepairManifestRepositories(branches, !c.Push, c.Force); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Create git branches for new branch. Exclude the ManifestProjects, which we just updated.
	if err = branch.CreateRemoteBranchesApi(authedClient, branch.GetNonManifestBranches(projectBranches), !c.Push, c.gerritWriteQps); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Bump version.
	if err = branch.BumpForCreate(componentToBump, c.release, c.Push, branchName, sourceUpstream); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	if !c.Push {
		branch.LogOut("Dry run (no --push): completed successfully")
	}
	return 0
}
