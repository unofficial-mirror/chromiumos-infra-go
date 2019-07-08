// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"github.com/maruel/subcommands"
)

var cmdRenameBranch = &subcommands.Command{
	UsageLine: "rename <options> old new",
	ShortDesc: "Rename a branch.",
	LongDesc:  "Rename a branch.",
	CommandRun: func() subcommands.CommandRun {
		c := &renameBranchRun{}
		c.Init()
		return c
	},
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

	return 0
}
