// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"log"
	"os"

	"go.chromium.org/chromiumos/infra/go/internal/repo_util"
)

type CrosCheckout struct {
	initialized bool
	root        string
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
	repository := &repo_util.Repository{c.root}
	err := repository.SyncToFile(path, RepoToolPath)
	return err
}
