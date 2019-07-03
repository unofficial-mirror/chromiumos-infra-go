// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo_util

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"go.chromium.org/chromiumos/infra/go/internal/osutils"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
)

var (
	commandRunnerImpl commandRunner = realCommandRunner{}
)

type commandRunner interface {
	runCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error
}

type realCommandRunner struct{}

func (c realCommandRunner) runCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	cmd.Dir = dir
	return cmd.Run()
}

type Repository struct {
	Root string
}

func FindRepoCheckoutRoot(root string) string {
	return osutils.FindInPathParents(".repo", root, "", osutils.DirExists)
}

func Initialize(root, manifestUrl, repoToolPath string) (Repository, error) {
	if FindRepoCheckoutRoot(root) != "" {
		return Repository{}, fmt.Errorf("Cannot init in existing repo %s.", root)
	}
	cmdArgs := []string{"init", "--manifest-url", manifestUrl}
	repoDir := filepath.Join(root, ".repo")

	ctx := context.Background()
	var stdoutBuf, stderrBuf bytes.Buffer
	if err := commandRunnerImpl.runCommand(ctx, &stdoutBuf, &stderrBuf, root, repoToolPath, cmdArgs...); err != nil {
		// On failure, delete the .repo directory.
		os.RemoveAll(repoDir)
		log.Printf("Error from repo.\nstdout =\n%s\n\nstderr=\n%s", stdoutBuf.String(), stderrBuf.String())
		return Repository{}, err
	}

	return Repository{root}, nil
}

// Sync repo at Root to manifest at manifestPath.
func (r *Repository) SyncToFile(manifestPath, repoToolPath string) error {
	if FindRepoCheckoutRoot(r.Root) == "" {
		return fmt.Errorf("No repo initialized at %s.", r.Root)
	}
	manifestPath = osutils.Abs(manifestPath)
	if !osutils.PathExists(manifestPath) {
		return fmt.Errorf("Manifest path %s does not exist.", manifestPath)
	}

	cmdArgs := []string{"sync", "--manifest-name", manifestPath}
	ctx := context.Background()

	var stdoutBuf, stderrBuf bytes.Buffer
	if err := commandRunnerImpl.runCommand(ctx, &stdoutBuf, &stderrBuf, r.Root, repoToolPath, cmdArgs...); err != nil {
		log.Printf("Error from repo.\nstdout =\n%s\n\nstderr=\n%s", stdoutBuf.String(), stderrBuf.String())
		return err
	}

	return nil
}

// Manifest runs `repo manifest` in the repository Root and returns the results
// as a repo.Manifest struct.
func (r *Repository) Manifest(repoToolPath string) (repo.Manifest, error) {
	// This implementation is a bit circuitous.
	// Put simply, we want the results of `repo manifest` as a repo.Manifest struct.
	// repo.LoadManifestFromFile already does a lot of the heavy lifting --
	// it is able to follow and load a manifest's imports. However, it requires a
	// file path as input, and `repo manifest` prints the Root manifest's contents
	// to stdout. As a workaround, I write these contents to a temp file that I then
	// pass into repo.LoadManifestFromFile.
	tmpFile, err := ioutil.TempFile(r.Root, "manifest")
	if err != nil {
		return repo.Manifest{}, fmt.Errorf("tmp file could not be created: %s", err.Error())
	}
	defer os.Remove(tmpFile.Name())

	// Run `repo manifest` and save results to tmp file.
	cmdArgs := []string{"manifest"}
	ctx := context.Background()

	var stdoutBuf, stderrBuf bytes.Buffer
	if err := commandRunnerImpl.runCommand(ctx, &stdoutBuf, &stderrBuf, r.Root, repoToolPath, cmdArgs...); err != nil {
		log.Printf("Error from repo.\nstdout =\n%s\n\nstderr=\n%s", stdoutBuf.String(), stderrBuf.String())
		return repo.Manifest{}, err
	}
	// Write results of `repo manifest` to file. We do this here instead of
	// using the -o flag to make testing easier.
	err = ioutil.WriteFile(tmpFile.Name(), []byte(stdoutBuf.String()), 0644)
	if err != nil {
		return repo.Manifest{}, fmt.Errorf("could not write manifest to tmp file %s: %s.", tmpFile.Name(), err.Error())
	}
	// Load manifest and imports into repo.Manifest structs.
	manifestMap, err := repo.LoadManifestFromFile(tmpFile.Name())
	if err != nil {
		return repo.Manifest{}, nil
	}
	return *manifestMap[tmpFile.Name()], err
}
