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
	"strconv"
	"time"

	"github.com/maruel/subcommands"
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
	gitRetries         = 3
	gitTimeout         = 30 * time.Second
)

var (
	RepoToolPath     string
	workingManifest  repo.Manifest
	manifestCheckout string
	workerCount      int
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
	c.Flags.IntVar(&workerCount, "j", 1, "Number of jobs to run for parallel operations.")
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

type checkoutOptions struct {
	// If set, will get only this ref.
	// If not set, will get the full repo.
	ref string
	// To be used with the git clone --depth flag.
	depth int
}

// Get a local checkout of a particular project.
func getProjectCheckout(projectPath string, opts *checkoutOptions) (string, error) {
	projectUrl, err := projectFetchUrl(projectPath)

	if err != nil {
		return "", errors.Annotate(err, "failed to get project fetch url").Err()
	}
	return getProjectCheckoutFromUrl(projectUrl, opts)
}

func getProjectCheckoutFromUrl(projectUrl string, opts *checkoutOptions) (string, error) {
	checkoutDir, err := ioutil.TempDir("", "cros-branch-")
	if err != nil {
		return "", errors.Annotate(err, "tmp dir could not be created").Err()
	}

	if err := git.Init(checkoutDir, false); err != nil {
		return "", err
	}
	if err := git.AddRemote(checkoutDir, "origin", projectUrl); err != nil {
		return "", errors.Annotate(err, "could not add %s as remote", projectUrl).Err()
	}

	cmd := []string{"fetch", "origin"}
	if opts != nil {
		if opts.ref != "" {
			cmd = append(cmd, git.StripRefs(opts.ref))
		}
		if opts.depth > 0 {
			cmd = append(cmd, "--depth", strconv.Itoa(opts.depth))
		}
	}
	output, err := git.RunGit(checkoutDir, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %s", projectUrl, output.Stderr)
	}
	checkoutBranch := "master"
	if opts != nil && opts.ref != "" {
		checkoutBranch = git.StripRefs(opts.ref)
	}
	if err := git.Checkout(checkoutDir, checkoutBranch); err != nil {
		return "", fmt.Errorf("failed to checkout %s", checkoutBranch)
	}

	return checkoutDir, nil
}

func initWorkingManifest(c branchCommand, branch string) error {
	opts := &checkoutOptions{
		depth: 1,
		ref:   branch,
	}
	manifestCheckout, err := getProjectCheckoutFromUrl(c.getManifestUrl(), opts)
	if err != nil {
		return errors.Annotate(err, "could not checkout %s", c.getManifestUrl()).Err()
	}

	if branch != "" {
		err := git.Checkout(manifestCheckout, branch)
		if err != nil {
			return errors.Annotate(err, "failed to checkout branch %s of %s", branch, c.getManifestUrl()).Err()
		}
	}

	manifestPath := filepath.Join(manifestCheckout, "default.xml")

	// Read in manifest from file (and resolve includes).
	manifest, err := repo.LoadManifestFromFileWithIncludes(manifestPath)
	if err != nil {
		return errors.Annotate(err, "failed to load manifests").Err()
	}
	workingManifest = *manifest
	return nil
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
