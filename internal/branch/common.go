// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package branch

import (
	"fmt"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
	"io/ioutil"
	"log"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const VersionFileProjectPath = "src/third_party/chromiumos-overlay"

var (
	StdoutLog        *log.Logger
	StderrLog        *log.Logger
	WorkingManifest  repo.Manifest
	RepoToolPath     string
	ManifestCheckout string
)

// CheckoutOptions describes how to check out a Git repo.
type CheckoutOptions struct {
	// If set, will get only this Ref.
	// If not set, will get the full repo.
	Ref string
	// To be used with the git clone --depth flag.
	Depth int
}

// LogOut logs to stdout.
func LogOut(format string, a ...interface{}) {
	if StdoutLog != nil {
		StdoutLog.Printf(format, a...)
	}
}

// LogOut logs to stderr.
func LogErr(format string, a ...interface{}) {
	if StderrLog != nil {
		StderrLog.Printf(format, a...)
	}
}

// ProjectFetchUrl returns the fetch URL for a remote Project.
func ProjectFetchUrl(projectPath string) (string, error) {
	project, err := WorkingManifest.GetProjectByPath(projectPath)
	if err != nil {
		return "", err
	}

	remote := WorkingManifest.GetRemoteByName(project.RemoteName)
	if remote == nil {
		return "", fmt.Errorf("remote %s does not exist in working manifest", project.RemoteName)
	}
	projectUrl, err := url.Parse(remote.Fetch)
	if err != nil {
		return "", errors.Annotate(err, "failed to parse fetch location for remote %s", remote.Name).Err()
	}
	projectUrl.Path = path.Join(projectUrl.Path, project.Name)

	return projectUrl.String(), nil
}

func getProjectCheckoutFromUrl(projectUrl string, opts *CheckoutOptions) (string, error) {
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
		if opts.Ref != "" {
			cmd = append(cmd, git.StripRefs(opts.Ref))
		}
		if opts.Depth > 0 {
			cmd = append(cmd, "--depth", strconv.Itoa(opts.Depth))
		}
	}
	output, err := git.RunGit(checkoutDir, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %s", projectUrl, output.Stderr)
	}
	checkoutBranch := ""
	if opts != nil && opts.Ref != "" {
		checkoutBranch = git.StripRefs(opts.Ref)
	} else {
		remoteBranch, err := git.ResolveRemoteSymbolicRef(checkoutDir, "origin", "HEAD")
		if err != nil {
			return "", fmt.Errorf("unable to resolve %s HEAD: %s", projectUrl, err)
		}
		parts := strings.Split(remoteBranch, "/")
		checkoutBranch = parts[len(parts)-1]
	}
	err = git.Checkout(checkoutDir, checkoutBranch)
	if err != nil {
		return "", fmt.Errorf("failed to checkout %s", checkoutBranch)
	}

	return checkoutDir, nil
}

// GetProjectCheckout gets a local checkout of a particular project.
func GetProjectCheckout(projectPath string, opts *CheckoutOptions) (string, error) {
	projectUrl, err := ProjectFetchUrl(projectPath)

	if err != nil {
		return "", errors.Annotate(err, "failed to get project fetch url").Err()
	}
	return getProjectCheckoutFromUrl(projectUrl, opts)
}

// InitWorkingManifest initializes a local working manifest (a.k.a. buildspec)
// from a Gerrit path.
func InitWorkingManifest(manifestUrl, br string) error {
	opts := &CheckoutOptions{
		Depth: 1,
		Ref:   br,
	}
	var err error
	ManifestCheckout, err = getProjectCheckoutFromUrl(manifestUrl, opts)
	if err != nil {
		return errors.Annotate(err, "could not checkout %s", manifestUrl).Err()
	}

	if br != "" {
		err := git.Checkout(ManifestCheckout, br)
		if err != nil {
			return errors.Annotate(err, "failed to checkout br %s of %s", br, manifestUrl).Err()
		}
	}

	manifestPath := filepath.Join(ManifestCheckout, "default.xml")

	// Read in manifest from file (and resolve includes).
	manifest, err := repo.LoadManifestFromFileWithIncludes(manifestPath)
	if err != nil {
		return errors.Annotate(err, "failed to load manifests").Err()
	}
	WorkingManifest = *manifest
	return nil
}
