// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"github.com/maruel/subcommands"
)

var cmdDeleteBranch = &subcommands.Command{
	UsageLine: "delete <options> branch_name",
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
	branch_name string
}

func (c *deleteBranchRun) validate(args []string) (bool, string) {
	if len(args) < 1 {
		return false, "missing required argument(s)."
	} else {
		c.branch_name = args[0]
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

	return 0
}
