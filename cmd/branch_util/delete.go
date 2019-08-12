// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

var cmdDeleteBranch = &subcommands.Command{
	UsageLine: "delete <options> branchName",
	ShortDesc: "Delete a branch.",
	LongDesc:  "Delete a branch.",
	CommandRun: func() subcommands.CommandRun {
		c := &deleteBranchRun{}
		c.Init()
		return c
	},
}

type deleteBranchRun struct {
	CommonFlags
	// Name of the branch to delete.
	branchName string
}

func (c *deleteBranchRun) validate(args []string) (bool, string) {
	if len(args) < 1 {
		return false, "missing required argument(s)."
	} else {
		c.branchName = args[0]
	}
	return true, ""
}

// Getters so that functions using the branchCommand interface
// can access CommonFlags in the underlying struct.
func (c *deleteBranchRun) getRoot() string {
	return c.Root
}

func (c *deleteBranchRun) getManifestUrl() string {
	return c.ManifestUrl
}

func (c *deleteBranchRun) Run(a subcommands.Application, args []string,
	env subcommands.Env) int {
	ret := Run(c, a, args, env)
	if ret != 0 {
		return ret
	}

	if c.Push && !c.Force {
		fmt.Fprintf(a.GetErr(), "Must set --force to delete remote branches.")
		return 1
	}

	var err error
	manifestCheckout, err := ioutil.TempDir("", "cros-branch-")
	if err != nil {
		fmt.Fprintf(a.GetErr(), errors.Annotate(err, "tmp dir could not be created").Err().Error()+"\n")
		return 1
	}
	defer os.RemoveAll(manifestCheckout)

	// Need to do this for testing, sadly -- don't want to delete real branches.
	if c.ManifestUrl != defaultManifestUrl {
		fmt.Fprintf(a.GetOut(), "Warning: --manifest-url should not be used for branch deletion.\n")
	}

	// Create local checkout of the manifest repo.
	_, err = git.RunGit(filepath.Dir(manifestCheckout),
		[]string{"clone", c.ManifestUrl, manifestCheckout, "--single-branch", "--branch", c.branchName})
	if err != nil {
		fmt.Fprintf(a.GetErr(), "Failed to clone %s of manifest repository.", c.branchName)
		return 1
	}
	manifestPath := filepath.Join(manifestCheckout, "default.xml")

	// Read in manifest from file (and resolve includes).
	manifest, err := repo.LoadManifestFromFileWithIncludes(manifestPath)
	if err != nil {
		err = errors.Annotate(err, "failed to load manifests").Err()
		fmt.Fprintf(a.GetErr(), err.Error())
		return 1
	}
	workingManifest = *manifest

	// Generate git branch names.
	branches := projectBranches(c.branchName, "")

	// Delete branches on remote.
	retCode := 0
	for _, projectBranch := range branches {
		project := projectBranch.project
		branch := git.NormalizeRef(projectBranch.branchName)
		remote := manifest.GetRemoteByName(project.RemoteName)
		projectRemote := fmt.Sprintf("%s/%s", remote.Fetch, project.Name)
		cmd := []string{"push", projectRemote, "--delete", branch}
		if !c.Push {
			cmd = append(cmd, "--dry-run")
		}

		_, err := git.RunGit(manifestCheckout, cmd)
		if err != nil {
			fmt.Fprintf(a.GetErr(), "Failed to delete branch %s in project %s.\n", branch, project.Name)
			// Try and delete as many of the branches as possible, even if some fail.
			retCode = 1
		}
	}

	return retCode
}
