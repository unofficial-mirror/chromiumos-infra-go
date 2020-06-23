// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"go.chromium.org/chromiumos/infra/go/internal/branch"
	"go.chromium.org/luci/auth"
	"os"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/git"
)

func getCmdRenameBranch(opts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "rename <options> old new",
		ShortDesc: "Rename a branch.",
		LongDesc:  "Rename a branch.",
		CommandRun: func() subcommands.CommandRun {
			c := &renameBranchRun{}
			c.InitFlags(opts)
			return c
		},
	}
}

type renameBranchRun struct {
	CommonFlags
	// Branch to rename.
	old string
	// New name for the branch.
	new string
}

func (c *renameBranchRun) validate(args []string) (bool, string) {
	if len(args) < 2 {
		return false, "missing required argument(s)."
	} else {
		c.old = args[0]
		c.new = args[1]
	}
	return true, ""
}

// Getters so that functions using the branchCommand interface
// can access CommonFlags in the underlying struct.
func (c *renameBranchRun) getRoot() string {
	return c.Root
}

func (c *renameBranchRun) getManifestUrl() string {
	return c.ManifestUrl
}

func (c *renameBranchRun) Run(a subcommands.Application, args []string,
	env subcommands.Env) int {
	ret := Run(c, a, args, env)
	if ret != 0 {
		return ret
	}

	if err := branch.InitWorkingManifest(c.getManifestUrl(), c.old); err != nil {
		branch.LogErr("%s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(branch.ManifestCheckout)

	// There is no way to atomically rename a remote branch. This method
	// creates new branches and deletes the old ones using portions of
	// the create and delete operations.

	// Need to do this for testing, sadly -- don't want to rename real branches.
	if c.ManifestUrl != defaultManifestUrl {
		branch.LogErr("Warning: --manifest-url should not be used for branch renaming.\n")
	}

	// Generate new git branch names.
	newBranches := projectBranches(c.new, c.old)

	// If not --force, validate branch names to ensure that they do not already exist.
	if !c.Force {
		err := assertBranchesDoNotExist(newBranches)
		if err != nil {
			branch.LogErr(err.Error())
			return 1
		}
	}

	// Repair manifest repositories.
	if err := repairManifestRepositories(newBranches, !c.Push, c.Force); err != nil {
		branch.LogErr(err.Error())
		return 1
	}
	// Create git branches for new branch.
	if err := createRemoteBranches(newBranches, !c.Push, c.Force); err != nil {
		branch.LogErr(err.Error())
		return 1
	}

	// Delete old branches.
	// TODO(@owner): Consider parallelizing this. It's not super important
	// because rename is seldom used.
	oldBranches := projectBranches(c.old, c.old)
	retCode := 0
	for _, projectBranch := range oldBranches {
		project := projectBranch.project
		br := git.NormalizeRef(projectBranch.branchName)
		remote := branch.WorkingManifest.GetRemoteByName(project.RemoteName)
		if remote == nil {
			// Try and delete as many of the branches as possible, even if some fail.
			branch.LogErr("Remote %s does not exist in working manifest.\n", project.RemoteName)
			retCode = 1
			continue
		}
		projectRemote := fmt.Sprintf("%s/%s", remote.Fetch, project.Name)
		cmd := []string{"push", projectRemote, "--delete", br}
		if !c.Push {
			cmd = append(cmd, "--dry-run")
		}

		_, err := git.RunGit(branch.ManifestCheckout, cmd)
		if err != nil {
			branch.LogErr("Failed to delete branch %s in project %s.\n", br, project.Name)
			// Try and delete as many of the branches as possible, even if some fail.
			retCode = 1
		}
	}

	return retCode
}
