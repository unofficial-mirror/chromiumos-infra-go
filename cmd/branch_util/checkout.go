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