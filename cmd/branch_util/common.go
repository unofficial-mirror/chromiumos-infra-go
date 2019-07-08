// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"io/ioutil"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/errors"
)

type branchCommand interface {
	validate([]string) (bool, string)
	getRoot() string
	getManifestUrl() string
}

// Common flags
type CommonFlags struct {
	subcommands.CommandRunBase
	Push        bool
	Force       bool
	Root        string
	ManifestUrl string
}

var (
	RepoToolPath string
	checkout     CrosCheckout
)

func (c *CommonFlags) Init() {
	// Common flags
	c.Flags.BoolVar(&c.Push, "push", false,
		"Push branch modifications to remote repos. Before setting this flag, "+
			"ensure that you have the proper permissions and that you know what "+
			"you are doing. Ye be warned.")
	c.Flags.BoolVar(&c.Force, "force", false,
		"Required for any remote operation that would delete an existing "+
			"branch. Also required when trying to branch from a previously "+
			"branched manifest version.")
	// Sync options
	c.Flags.StringVar(&c.Root, "root", "",
		"Repo root of local checkout to branch. If the root does not "+
			"exist, this tool will create it. If the root is not initialized, "+
			"this tool will initialize it. If --root is not specificed, this "+
			"tool will branch a fresh checkout in a temporary directory.")
	c.Flags.StringVar(&c.ManifestUrl, "manifest_url",
		"https://chrome-internal.googlesource.com/chromeos/manifest-internal.git",
		"URL of the manifest to be checked out. Defaults to googlesource URL "+
			"for manifest-internal.")
}

func Run(c branchCommand, a subcommands.Application, args []string,
	// Validate flags/arguments.
	env subcommands.Env) int {
	ok, errMsg := c.validate(args)
	if !ok {
		fmt.Fprintf(a.GetErr(), errMsg+"\n")
		return 1
	}

	var err error
	root := c.getRoot()
	if root == "" {
		root, err = ioutil.TempDir("", "cros-branch-")
		// TODO(jackneus): Delete tmp dir at end.
		if err != nil {
			fmt.Fprintf(a.GetErr(), errors.Annotate(err, "tmp root could not be created").Err().Error()+"\n")
			return 1
		}
	}
	checkout.Initialize(root, c.getManifestUrl())

	return 0
}
