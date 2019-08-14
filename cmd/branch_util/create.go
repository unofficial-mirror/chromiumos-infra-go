// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

var (
	skipSync bool
)

const (
	versionProjectPath = "src/third_party/chromiumos-overlay"
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
func (c *createBranchRun) newBranchName(vinfo repo.VersionInfo) string {
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

func (c *createBranchRun) bumpVersion(
	component repo.VersionComponent,
	branch, commitMsg string,
	dryRun bool) error {
	// Get checkout of versionProjectPath, which has chromeos_version.sh.
	versionProjectCheckout, err := getProjectCheckout(versionProjectPath)
	defer os.RemoveAll(versionProjectCheckout)
	if err != nil {
		return errors.Annotate(err, "local checkout of version project failed").Err()
	}

	// Branch won't exist if running tool with --dry-run.
	if !dryRun {
		if err := git.Checkout(versionProjectCheckout, branch); err != nil {
			return errors.Annotate(err, "failed to checkout branch %s", branch).Err()
		}
	}

	version, err := repo.GetVersionInfoFromRepo(versionProjectCheckout)
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
	errs := []error{
		err,
		git.PushRef(versionProjectCheckout, "HEAD", dryRun, remoteRef),
	}
	for _, err := range errs {
		if err != nil {
			return errors.Annotate(err, "failed to push version changes to remote").Err()
		}
	}
	return nil
}

func (c *createBranchRun) Run(a subcommands.Application, args []string,
	env subcommands.Env) int {
	// Common setup (argument validation, repo init, etc.)
	ret := Run(c, a, args, env)
	if ret != 0 {
		return ret
	}
	if err := initWorkingManifest(c, ""); err != nil {
		fmt.Fprintf(a.GetErr(), "%s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(manifestCheckout)

	// Validate the version.
	// Double check that the checkout has a zero patch number. Otherwise,
	// we cannot branch from it.
	versionProjectCheckout, err := getProjectCheckout(versionProjectPath)
	defer os.RemoveAll(versionProjectCheckout)
	if err != nil {
		err = errors.Annotate(err, "local checkout of version project failed").Err()
		fmt.Fprintf(a.GetErr(), "%s\n", err.Error())
		return 1
	}

	vinfo, err := repo.GetVersionInfoFromRepo(versionProjectCheckout)
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
	manifestInternal, err := workingManifest.GetUniqueProject("chromeos/manifest-internal")
	if err != nil {
		fmt.Fprintf(a.GetErr(),
			errors.Annotate(err, "Could not get chromeos/manifest-internal project.").Err().Error())
		return 1
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`.*-%s.B$`, vinfo.StrippedVersionString()))
	exists, err := branchExists(manifestInternal, pattern)
	if err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}
	if exists && !c.Force {
		fmt.Fprintf(a.GetErr(), "Already branched %s. Please rerun with --force if you "+
			"would like to proceed.", vinfo.VersionString())
		return 1
	}

	// Generate branch name.
	branchName := c.newBranchName(vinfo)

	// TODO(@jackneus): double check name with user via boolean CLI prompt

	// Create branch.

	componentToBump, err := whichVersionShouldBump(vinfo)
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

	// Create git branches for new branch.
	if err = createRemoteBranches(branches, !c.Push, c.Force); err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}
	// Repair manifest repositories.
	if err = repairManifestRepositories(branches, !c.Push, c.Force); err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}

	// Bump version.
	commitMsg := fmt.Sprintf("Bump %s number after creating branch %s.", componentToBump, branchName)
	if err = c.bumpVersion(componentToBump, branchName, commitMsg, !c.Push); err != nil {
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}
	// Increment branch/build number for source 'master' branch.
	// crbug.com/965164
	// TODO(@jackneus): refactor
	if c.release {
		commitMsg = fmt.Sprintf("Bump milestone after creating release branch %s.", branchName)
		if err = c.bumpVersion(repo.ChromeBranch, "master", commitMsg, !c.Push); err != nil {
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
		revision := workingManifest.Default.Revision
		if revision == "" {
			revision = git.NormalizeRef("master")
		}
		err = c.bumpVersion(sourceComponentToBump, revision, commitMsg, !c.Push)
		if err != nil {
			fmt.Fprintf(a.GetErr(), err.Error())
			return 1
		}
	}

	return 0
}
