// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test_util

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/util"
	"go.chromium.org/luci/common/errors"
)

var (
	CommandRunnerImpl cmd.CommandRunner = cmd.RealCommandRunner{}
)

// AssertContentsEqual checks that there's no difference between two directories/files.
func AssertContentsEqual(path_a, path_b string) error {
	ai, err := os.Stat(path_a)
	if err != nil {
		return errors.Annotate(err, "failed to stat %s", path_a).Err()
	}
	bi, err := os.Stat(path_b)
	if err != nil {
		return errors.Annotate(err, "failed to stat %s", path_b).Err()
	}
	// diff -rq works for dirs and files, but only if both operands are the same.
	if ai.IsDir() && !bi.IsDir() {
		return fmt.Errorf("%s is a dir and %s is a file", path_a, path_b)
	}
	if !ai.IsDir() && bi.IsDir() {
		return fmt.Errorf("%s is a file and %s is a dir", path_a, path_b)
	}

	ctx := context.Background()
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := []string{"-rq", path_a, path_b}
	if err = CommandRunnerImpl.RunCommand(ctx, &stdoutBuf, &stderrBuf, "", "diff", cmd...); err != nil {
		if err.Error() == "exit status 1" {
			return fmt.Errorf("files differ")
		} else {
			return fmt.Errorf("error running diff")
		}
	}
	return err
}

// AssertGitBranches asserts that the git repo has the given branches (it may have others, too).
func AssertGitBranches(gitRepo string, branches []string) error {
	actual, err := git.MatchBranchNameWithNamespace(gitRepo, regexp.MustCompile(".*"), regexp.MustCompile("refs/heads/"))
	if err != nil {
		return errors.Annotate(err, "error getting branches").Err()
	}
	if !util.UnorderedContains(actual, branches) {
		return fmt.Errorf("project branch mismatch. expected: %v got %v", branches, actual)
	}
	return nil
}

// AssertGitBranches asserts that the git repo has only the correct branches.
func AssertGitBranchesExact(gitRepo string, branches []string) error {
	actual, err := git.MatchBranchNameWithNamespace(gitRepo, regexp.MustCompile(".*"), regexp.MustCompile("refs/heads/"))
	if err != nil {
		return errors.Annotate(err, "error getting branches").Err()
	}
	// Remove duplicates from branches. This is OK because branch names are unique identifiers
	// and so having a branch name twice in branches doesn't mean anything special.
	branchMap := make(map[string]bool)
	for _, branch := range branches {
		branchMap[branch] = true
	}
	branches = []string{}
	for branch := range branchMap {
		branches = append(branches, branch)
	}
	if !util.UnorderedEqual(actual, branches) {
		return fmt.Errorf("project branch mismatch. expected: %v got %v", branches, actual)
	}
	return nil
}
