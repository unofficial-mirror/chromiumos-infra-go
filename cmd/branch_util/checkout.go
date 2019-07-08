// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/chromiumos/infra/go/internal/repo_util"
)

type CrosCheckout struct {
	initialized bool
	root        string
	Manifest    repo.Manifest
}

func (c *CrosCheckout) Initialize(root, manifestUrl string) error {
	if root == "" {
		return fmt.Errorf("Root is null.")
	}

	os.MkdirAll(root, 0775)
	// Check to see if repo has been initialized.
	repoPath := repo_util.FindRepoCheckoutRoot(root)
	if repoPath == "" {
		// Initialize repo.
		log.Printf("Will initialize checkout %s for this run.", root)
		_, err := repo_util.Initialize(root, manifestUrl, RepoToolPath)
		if err != nil {
			return err
		}
	} else {
		// Repo has already been initialized.
		log.Printf("Will use existing checkout %s for this run.", root)
	}
	c.root = root
	c.initialized = true
	return nil
}

func (c *CrosCheckout) SyncToManifest(path string) error {
	if !c.initialized {
		return fmt.Errorf("Checkout has not been initialized.")
	}
	log.Printf("Syncing checkout %s to manifest %s.", c.root, path)
	repository := &repo_util.Repository{Root: c.root}
	if !skipSync {
		if err := repository.SyncToFile(path, RepoToolPath); err != nil {
			return err
		}
	}
	var err error
	c.Manifest, err = repository.Manifest(RepoToolPath)
	return err
}

func (c *CrosCheckout) ReadVersion() repo.VersionInfo {
	vinfo, err := repo.GetVersionInfoFromRepo(c.root)
	if err != nil {
		return repo.VersionInfo{}
	}
	return vinfo
}

// AbsolutePath joins the path components with the repo root.
func (c *CrosCheckout) AbsolutePath(args ...string) string {
	args = append([]string{c.root}, args...)
	return filepath.Join(args...)
}

// AbsoluteProjectPath joins the path components with the project's root.
func (c *CrosCheckout) AbsoluteProjectPath(project repo.Project, args ...string) string {
	args = append([]string{project.Path}, args...)
	return c.AbsolutePath(args...)
}

// BranchExists determines if any branch exists in the specified project
// that matches the specified pattern.
func (c *CrosCheckout) BranchExists(project repo.Project, pattern *regexp.Regexp) (bool, error) {
	matches, err := git.MatchBranchName(c.AbsoluteProjectPath(project), pattern)
	return len(matches) != 0, err
}
