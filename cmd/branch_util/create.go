// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"os"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/branch"
	mv "go.chromium.org/chromiumos/infra/go/internal/chromeos_version"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
)

func getCmdCreateBranchV1(opts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create-v1 <options>",
		ShortDesc: "Create a branch(DEPRECATED).",
		LongDesc:  "Create a branch(DEPRECATED).",
		CommandRun: func() subcommands.CommandRun {
			c := &createBranchRunV1{}
			c.InitFlags(opts)
			// Arguments for determining branch name.
			c.Flags.StringVar(&c.descriptor, "descriptor", "",
				"Optional descriptor for this branch. Typically, this is a build "+
					"target or a device, depending on the nature of the branch. Used "+
					"to generate the branch name. Cannot be used with --custom.")
			// Which manifest should be branched?
			c.Flags.StringVar(&c.version, "version", "",
				"Manifest version to branch off, e.g. '10509.0.0'. You may not branch "+
					"off of the same version twice unless you run "+
					"with --force.")
			c.Flags.StringVar(&c.file, "file", "", "Path to manifest file to branch off.")
			// What kind of branch is this?
			// TODO(@owner): Figure out how to group these flags in the
			// help dialog. Right now all flags are displayed in alphabetic
			// order, which is less helpful.
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

type createBranchRunV1 struct {
	CommonFlags
	yes        bool
	descriptor string
	version    string
	file       string
	release    bool
	factory    bool
	firmware   bool
	stabilize  bool
	custom     string
}

func (c *createBranchRunV1) validate(args []string) (bool, string) {
	if c.file == "" && c.version == "" || c.file != "" && c.version != "" {
		return false, "must set exactly one of --file or --version."
	}
	_, ok := branch.BranchType(c.release, c.factory, c.firmware, c.stabilize, c.custom)
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
func (c *createBranchRunV1) getRoot() string {
	return c.Root
}

func (c *createBranchRunV1) getManifestUrl() string {
	return c.ManifestUrl
}

func (c *createBranchRunV1) Run(a subcommands.Application, args []string,
	env subcommands.Env) int {
	// Common setup (argument validation, repo init, etc.)
	ret := Run(c, a, args, env)
	if ret != 0 {
		return ret
	}
	if c.file != "" {
		// Branch from file.
		manifest, err := repo.LoadManifestFromFileWithIncludes(c.file)
		if err != nil {
			err = errors.Annotate(err, "failed to load manifests").Err()
			branch.LogErr("%s\n", err.Error())
			return 1
		}
		branch.WorkingManifest = *manifest
	} else {
		branch.LogOut("Downloading and parsing buildspec manifest for %s. This may take a few minutes.\n", c.version)
		manifest, err := branch.GetWorkingManifestForVersion(c.version)
		if err != nil {
			err = errors.Annotate(err, "failed to load manifests").Err()
			branch.LogErr("%s\n", err.Error())
			return 2
		}
		branch.WorkingManifest = *manifest
	}
	branch.LogOut("Fetched working manifest.\n")

	// Use manifest-internal as a sentinel repository to get the appropriate branch name.
	// We know that manifest-internal is a single-checkout so its revision should be
	// master or the name of the Chrome OS branch.
	// TODO(@jackneus): write test for this.
	manifestInternal, err := branch.WorkingManifest.GetUniqueProject("chromeos/manifest-internal")
	if err != nil {
		branch.LogErr(errors.Annotate(err, "Could not get chromeos/manifest-internal project.").Err().Error())
		return 3
	}
	sourceRevision := manifestInternal.Revision
	sourceUpstream := git.StripRefs(manifestInternal.Upstream)
	branch.LogErr("Using sourceRevision %s for manifestInternal", sourceRevision)
	branch.LogErr("Using sourceUpstream %s for manifestInternal", sourceUpstream)

	// Validate the version.
	// Double check that the checkout has a zero patch number. Otherwise,
	// we cannot branch from it.
	versionProject, err := branch.WorkingManifest.GetProjectByPath(branch.VersionFileProjectPath)
	if err != nil {
		err = errors.Annotate(err, "could not get project %s from manifest", branch.VersionFileProjectPath).Err()
		branch.LogErr("%s\n", err)
		return 4
	}
	opts := &branch.CheckoutOptions{
		Depth: 1,
		Ref:   git.StripRefs(versionProject.Revision),
	}
	versionProjectCheckout, err := branch.GetProjectCheckout(branch.VersionFileProjectPath, opts)
	defer os.RemoveAll(versionProjectCheckout)
	if err != nil {
		err = errors.Annotate(err, "local checkout of version project failed").Err()
		branch.LogErr("%s\n", err.Error())
		return 5
	}

	vinfo, err := mv.GetVersionInfoFromRepo(versionProjectCheckout)
	if err != nil {
		branch.LogErr(errors.Annotate(err, "error reading version").Err().Error())
		return 6
	}
	if vinfo.PatchNumber != 0 {
		branch.LogErr("Cannot branch version with nonzero patch number (version %s).",
			vinfo.VersionString())
		return 7
	}
	branch.LogOut("Version found: %s.\n", vinfo.VersionString())

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
		return 10
	}

	// Generate git branch names.
	branches := branch.ProjectBranches(branchName, git.StripRefs(sourceRevision))
	branch.LogOut("Creating branch: %s\n", branchName)

	// If not --force, validate branch names to ensure that they do not already exist.
	if !c.Force {
		err = branch.AssertBranchesDoNotExist(branches, workerCount)
		if err != nil {
			branch.LogErr(err.Error())
			return 11
		}
	}
	branch.LogOut("Done validating project branches.\n")

	// Repair manifest repositories.
	if err = branch.RepairManifestRepositories(branches, !c.Push, c.Force); err != nil {
		branch.LogErr(err.Error())
		return 12
	}
	// Create git branches for new branch.
	if err = branch.CreateRemoteBranches(branches, !c.Push, c.Force, workerCount); err != nil {
		branch.LogErr(err.Error())
		return 13
	}

	// Bump version.
	if err = branch.BumpForCreate(componentToBump, c.release, c.Push, branchName, sourceUpstream); err != nil {
		branch.LogErr(err.Error())
		return 14
	}

	return 0
}
