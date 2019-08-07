// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

var (
	skipSync bool
)

var cmdCreateBranch = &subcommands.Command{
	UsageLine: "create <options>",
	ShortDesc: "Create a branch.",
	LongDesc:  "Create a branch.",
	CommandRun: func() subcommands.CommandRun {
		c := &createBranchRun{}
		c.Init()
		c.Flags.BoolVar(&c.yes, "yes", false,
			"If set, disables the boolean prompt confirming the branch name.")
		// Arguments for determining branch name.
		c.Flags.StringVar(&c.descriptor, "descriptor", "",
			"Optional descriptor for this branch. Typically, this is a build "+
				"target or a device, depending on the nature of the branch. Used "+
				"to generate the branch name. Cannot be used with --custom.")
		// Which manifest should be branched?
		// TODO(@jackneus): Implement version logic
		//c.Flags.StringVar(&c.version, "version", "",
		//	"Manifest version to branch off, e.g. '10509.0.0'. You may not branch "+
		//		"off of the same version twice unless you run "+
		//		"with --force.")
		c.Flags.StringVar(&c.file, "file", "", "Path to manifest file to branch off.")
		// What kind of branch is this?
		// TODO(@jackneus): Figure out how to group these flags in the
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

type createBranchRun struct {
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

func (c *createBranchRun) getBranchType() (string, bool) {
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

func (c *createBranchRun) validate(args []string) (bool, string) {
	if c.file == "" {
		return false, "must set --file."
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
func (c *createBranchRun) getRoot() string {
	return c.Root
}

func (c *createBranchRun) getManifestUrl() string {
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
func (c *createBranchRun) newBranchName() string {
	if c.custom != "" {
		return c.custom
	}
	vinfo, _ := checkout.ReadVersion()
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

func (c *createBranchRun) Run(a subcommands.Application, args []string,
	env subcommands.Env) int {
	// Common setup (argument validation, repo init, etc.)
	ret := Run(c, a, args, env)
	if ret != 0 {
		return ret
	}

	// Sync repo to manifest at provided path.
	err := checkout.SyncToManifest(c.file)
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err.Error())
		return 1
	}

	// Validate the version.
	// Double check that the checkout has a zero patch number. Otherwise,
	// we cannot branch from it.
	vinfo, err := checkout.ReadVersion()
	if err != nil {
		fmt.Fprintf(a.GetErr(), errors.Annotate(err, "error reading version").Err().Error())
		return 1
	}
	if vinfo.PatchNumber != 0 {
		fmt.Fprintf(a.GetErr(), "Cannot branch version with nonzero patch number (version %s).",
			vinfo.VersionString())
		return 1
	}

	// Check that we did not already branch from this version.
	// manifest-internal serves as the sentinel project.
	manifest := checkout.Manifest()
	manifestInternal, err := manifest.GetUniqueProject("chromeos/manifest-internal")
	if err != nil {
		fmt.Fprintf(a.GetErr(),
			errors.Annotate(err, "Could not get chromeos/manifest-internal project.").Err().Error())
		return 1
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`.*-%s.B$`, vinfo.StrippedVersionString()))
	branchExists, err := checkout.BranchExists(manifestInternal, pattern)
	if err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}
	if branchExists && !c.Force {
		fmt.Fprintf(a.GetErr(), "Already branched %s. Please rerun with --force if you "+
			"would like to proceed.", vinfo.VersionString())
	}

	// Generate branch name.
	branchName := c.newBranchName()

	// TODO(@jackneus): double check name with user via boolean CLI prompt

	// Create branch.

	componentToBump, err := whichVersionShouldBump()
	if err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}

	// Generate git branch names.
	branches := projectBranches(branchName, "")
	// If not --force, validate branch names to ensure that they do not already exist.
	if !c.Force {
		err = assertBranchesDoNotExist(branches)
		if err != nil {
			fmt.Fprintf(a.GetErr(), err.Error())
			return 1
		}
	}

	// Repair manifest repositories.
	if err = repairManifestRepositories(branches, !c.Push, c.Force); err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}

	// Bump version.
	commitMsg := fmt.Sprintf("Bump %s number after creating branch %s.", componentToBump, branchName)
	if err = checkout.BumpVersion(componentToBump, branchName, commitMsg, !c.Push, false); err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}
	// Increment branch/build number for source 'master' branch.
	// crbug.com/965164
	// TODO(@jackneus): refactor
	if c.release {
		commitMsg = fmt.Sprintf("Bump milestone after creating release branch %s.", branchName)
		if err = checkout.BumpVersion(repo.ChromeBranch, "master", commitMsg, !c.Push, true); err != nil {
			fmt.Fprintf(a.GetErr(), err.Error())
			return 1
		}
	} else {
		var sourceComponentToBump repo.VersionComponent
		if componentToBump == repo.Patch {
			sourceComponentToBump = repo.Branch
		} else {
			sourceComponentToBump = repo.Build
		}
		commitMsg = fmt.Sprintf("Bump %s number for source branch after creating branch %s.",
			sourceComponentToBump, branchName)
		err = checkout.BumpVersion(sourceComponentToBump, manifest.Default.Revision, commitMsg, !c.Push, true)
		if err != nil {
			fmt.Fprintf(a.GetErr(), err.Error())
			return 1
		}
	}

	return 0
}
