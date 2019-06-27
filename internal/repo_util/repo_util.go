// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo_util

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"go.chromium.org/chromiumos/infra/go/internal/osutils"
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
	root string
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