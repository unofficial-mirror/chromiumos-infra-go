// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
)

var (
	CommandRunnerImpl cmd.CommandRunner = cmd.RealCommandRunner{}
)

type CommandOutput struct {
	Stdout string
	Stderr string
}

// Struct representing a remote ref.
type RemoteRef struct {
	Remote string
	Ref    string
}

// RunGit the specified git command in the specified repo. It returns
// stdout and stderr.
func RunGit(gitRepo string, cmd []string) (CommandOutput, error) {
	ctx := context.Background()
	var stdoutBuf, stderrBuf bytes.Buffer
	err := CommandRunnerImpl.RunCommand(ctx, &stdoutBuf, &stderrBuf, gitRepo, "git", cmd...)
	cmdOutput := CommandOutput{stdoutBuf.String(), stderrBuf.String()}
	return cmdOutput, err
}

// GetCurrentBranch returns current branch of a repo, and an empty string
// if repo is on detached HEAD.
func GetCurrentBranch(cwd string) string {
	output, err := RunGit(cwd, []string{"symbolic-ref", "-q", "HEAD"})
	if err != nil {
		return ""
	}
	return StripRefsHead(strings.TrimSpace(output.Stdout))
}

// MatchBranchName returns the names of branches who match the specified
// regular expression.
func MatchBranchName(gitRepo string, pattern *regexp.Regexp) ([]string, error) {
	// Regex should be case insensitive.
	if !strings.HasPrefix(pattern.String(), "(?i)") {
		pattern = regexp.MustCompile("(?i)" + pattern.String())
	}

	output, err := RunGit(gitRepo, []string{"ls-remote", gitRepo})
	if err != nil {
		// Could not read branches.
		return []string{}, fmt.Errorf("git error: %s\nstderr: %s", err.Error(), output.Stderr)
	}
	// Find all branches that match the pattern.
	branches := strings.Split(output.Stdout, "\n")
	matchedBranches := []string{}
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		branch = strings.Fields(branch)[1]
		if pattern.Match([]byte(branch)) {
			matchedBranches = append(matchedBranches, branch)
		}
	}
	return matchedBranches, nil
}

// GetGitRepoRevision finds and returns the revision of a branch.
func GetGitRepoRevision(cwd string) (string, error) {
	output, err := RunGit(cwd, []string{"rev-parse", "HEAD"})
	return strings.TrimSpace(output.Stdout), err
}

// StipRefsHead removes leading 'refs/heads/' from a ref name.
func StripRefsHead(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// NormalizeRef converts git branch refs into fully qualified form.
func NormalizeRef(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "refs/") {
		return ref
	}
	return fmt.Sprintf("refs/heads/%s", ref)
}

// StripRefs removes leading 'refs/heads/', 'refs/remotes/[^/]+/' from a ref name.
func StripRefs(ref string) string {
	ref = StripRefsHead(ref)
	// If the ref starts with ref/remotes/, then we want the part of the string
	// that comes after the third "/".
	// Example: refs/remotes/origin/master --> master
	// Example: refs/remotse/origin/foo/bar --> foo/bar
	if strings.HasPrefix(ref, "refs/remotes/") {
		refParts := strings.SplitN(ref, "/", 4)
		return refParts[len(refParts)-1]
	}
	return ref
}
