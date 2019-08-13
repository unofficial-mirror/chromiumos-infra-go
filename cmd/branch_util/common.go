// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"path/filepath"

	"github.com/maruel/subcommands"
	checkoutp "go.chromium.org/chromiumos/infra/go/internal/checkout"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
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

const (
	defaultManifestUrl = "https://chrome-internal.googlesource.com/chromeos/manifest-internal"
)

var (
	RepoToolPath    string
	checkout        checkoutp.Checkout
	workingManifest repo.Manifest
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
	c.Flags.StringVar(&c.ManifestUrl, "manifest-url", defaultManifestUrl,
		"URL of the manifest to be checked out. Defaults to googlesource URL "+
			"for manifest-internal.")
}

// projectFetchUrl returns the fetch URL for a remote project.
func projectFetchUrl(projectPath string) (string, error) {
	project, err := workingManifest.GetProjectByPath(projectPath)
	if err != nil {
		return "", err
	}

	remote := workingManifest.GetRemoteByName(project.RemoteName)
	projectUrl, err := url.Parse(remote.Fetch)
	if err != nil {
		return "", errors.Annotate(err, "failed to parse fetch location for remote %s", remote.Name).Err()
	}
	projectUrl.Path = path.Join(projectUrl.Path, project.Name)

	return projectUrl.String(), nil
}

// Get a local checkout of a particular project.
func getProjectCheckout(projectPath string) (string, error) {
	checkoutDir, err := ioutil.TempDir("", "cros-branch-")
	if err != nil {
		return "", errors.Annotate(err, "tmp dir could not be created").Err()
	}

	projectUrl, err := projectFetchUrl(projectPath)

	if err != nil {
		return "", errors.Annotate(err, "failed to get project fetch url").Err()
	}

	// TODO(@jackneus): add  "--branch", git.StripRefs(project.Upstream) when appropriate?
	output, err := git.RunGit(filepath.Dir(checkoutDir),
		[]string{"clone", projectUrl, checkoutDir})
	if err != nil {
		return "", fmt.Errorf("failed to clone %s: %s", projectUrl, output.Stderr)
	}

	return checkoutDir, nil
}

func Run(c branchCommand, a subcommands.Application, args []string,
	// Validate flags/arguments.
	env subcommands.Env) int {
	ok, errMsg := c.validate(args)
	if !ok {
		fmt.Fprintf(a.GetErr(), errMsg+"\n")
		return 1
	}
	return 0
}
