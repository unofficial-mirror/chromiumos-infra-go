// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
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

// RunGitIgnore the specified git command in the specified repo and returns
// only an error, not the command output.
func RunGitIgnoreOutput(gitRepo string, cmd []string) error {
	_, err := RunGit(gitRepo, cmd)
	return err
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
	// MatchBranchWithNamespace trims the namespace off the branches it returns.
	// Here, we need a namespace that matches every string but doesn't match any character
	// (so that nothing is trimmed).
	nullNamespace := regexp.MustCompile("")
	return MatchBranchNameWithNamespace(gitRepo, pattern, nullNamespace)
}

// MatchBranchNameWithNamespace returns the names of branches who match the specified
// pattern and start with the specified namespace.
func MatchBranchNameWithNamespace(gitRepo string, pattern, namespace *regexp.Regexp) ([]string, error) {
	// Regex should be case insensitive.
	namespace = regexp.MustCompile("(?i)^" + namespace.String())
	pattern = regexp.MustCompile("(?i)" + pattern.String())

	output, err := RunGit(gitRepo, []string{"show-ref"})
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			// Not a fatal error, just no branches.
			return []string{}, nil
		}
		// Could not read branches.
		return []string{}, fmt.Errorf("git error: %s\nstdout: %s stderr: %s", err.Error(), output.Stdout, output.Stderr)
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

		// Only look at branches which match the namespace.
		if !namespace.Match([]byte(branch)) {
			continue
		}
		branch = namespace.ReplaceAllString(branch, "")

		if pattern.Match([]byte(branch)) {
			matchedBranches = append(matchedBranches, branch)
		}
	}
	return matchedBranches, nil
}

// GetGitRepoRevision finds and returns the revision of a branch.
func GetGitRepoRevision(cwd, branch string) (string, error) {
	if branch == "" {
		branch = "HEAD"
	} else {
		branch = NormalizeRef(branch)
	}
	output, err := RunGit(cwd, []string{"rev-parse", branch})
	return strings.TrimSpace(output.Stdout), err
}

// IsReachable determines whether one commit ref is reachable from another.
func IsReachable(cwd, to_ref, from_ref string) (bool, error) {
	_, err := RunGit(cwd, []string{"merge-base", "--is-ancestor", to_ref, from_ref})
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// StripRefsHead removes leading 'refs/heads/' from a ref name.
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

// CreateBranch creates a branch.
func CreateBranch(gitRepo, branch string) error {
	output, err := RunGit(gitRepo, []string{"checkout", "-B", branch})
	if err != nil {
		if strings.Contains(output.Stderr, "not a valid branch name") {
			return fmt.Errorf("%s is not a valid branch name", branch)
		} else {
			return fmt.Errorf(output.Stderr)
		}
	}
	return err
}

// CreateTrackingBranch creates a tracking branch.
func CreateTrackingBranch(gitRepo, branch string, remoteRef RemoteRef) error {
	refspec := fmt.Sprintf("%s/%s", remoteRef.Remote, remoteRef.Ref)
	output, err := RunGit(gitRepo, []string{"fetch", remoteRef.Remote, remoteRef.Ref})
	if err != nil {
		return fmt.Errorf("could not fetch %s: %s", refspec, output.Stderr)
	}
	output, err = RunGit(gitRepo, []string{"checkout", "-b", branch, "-t", refspec})
	if err != nil {
		if strings.Contains(output.Stderr, "not a valid branch name") {
			return fmt.Errorf("%s is not a valid branch name", branch)
		} else {
			return fmt.Errorf(output.Stderr)
		}
	}
	return err
}

// CommitAll adds all local changes and commits them.
// Returns the sha1 of the commit.
func CommitAll(gitRepo, commitMsg string) (string, error) {
	if output, err := RunGit(gitRepo, []string{"add", "-A"}); err != nil {
		return "", fmt.Errorf(output.Stderr)
	}
	if output, err := RunGit(gitRepo, []string{"commit", "-m", commitMsg}); err != nil {
		if strings.Contains(output.Stdout, "nothing to commit") {
			return "", fmt.Errorf(output.Stdout)
		} else {
			return "", fmt.Errorf(output.Stderr)
		}
	}
	output, err := RunGit(gitRepo, []string{"rev-parse", "HEAD"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output.Stdout), nil
}

// CommitEmpty makes an empty commit (assuming nothing is staged).
// Returns the sha1 of the commit.
func CommitEmpty(gitRepo, commitMsg string) (string, error) {
	if output, err := RunGit(gitRepo, []string{"commit", "-m", commitMsg, "--allow-empty"}); err != nil {
		return "", fmt.Errorf(output.Stderr)
	}
	output, err := RunGit(gitRepo, []string{"rev-parse", "HEAD"})
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(output.Stdout), nil
}

// PushChanges stages and commits any local changes before pushing the commit
// to the specified remote ref. Returns the sha1 of the commit.
func PushChanges(gitRepo, localRef, commitMsg string, dryRun bool, pushTo RemoteRef) (string, error) {
	commit, err := CommitAll(gitRepo, commitMsg)
	// It's ok if there's nothing to commit, we can still try to push.
	if err != nil && !strings.Contains(err.Error(), "nothing to commit") {
		return "", err
	}
	return commit, PushRef(gitRepo, localRef, dryRun, pushTo)
}

// PushRef pushes the specified local ref to the specified remote ref.
func PushRef(gitRepo, localRef string, dryRun bool, pushTo RemoteRef) error {
	ref := fmt.Sprintf("%s:%s", localRef, pushTo.Ref)
	cmd := []string{"push", pushTo.Remote, ref}
	if dryRun {
		cmd = append(cmd, "--dry-run")
	}
	_, err := RunGit(gitRepo, cmd)
	return err
}

// Init initializes a repo.
func Init(gitRepo string, bare bool) error {
	cmd := []string{"init"}
	if bare {
		cmd = append(cmd, "--bare")
	}
	_, err := RunGit(gitRepo, cmd)
	return err
}

// AddRemote adds a remote.
func AddRemote(gitRepo, remote, remoteLocation string) error {
	output, err := RunGit(gitRepo, []string{"remote", "add", remote, remoteLocation})
	if err != nil {
		if strings.Contains(output.Stderr, "already exists") {
			return fmt.Errorf("remote already exists")
		}
	}
	return err
}

// Checkout checkouts a branch.
func Checkout(gitRepo, branch string) error {
	output, err := RunGit(gitRepo, []string{"checkout", branch})
	if err != nil && strings.Contains(output.Stderr, "did not match any") {
		return fmt.Errorf(output.Stderr)
	}
	return err
}

// DeleteBranch checks out to master and then deletes the current branch.
func DeleteBranch(gitRepo, branch string, force bool) error {
	cmd := []string{"branch"}
	if force {
		cmd = append(cmd, "-D")
	} else {
		cmd = append(cmd, "-d")
	}
	cmd = append(cmd, branch)
	output, err := RunGit(gitRepo, cmd)

	if err != nil {
		if strings.Contains(output.Stderr, "checked out at") {
			return fmt.Errorf(output.Stderr)
		}
		if strings.Contains(output.Stderr, "not fully merged") {
			return fmt.Errorf("branch %s is not fully merged. use the force parameter if you wish to proceed", branch)
		}
	}
	return err
}

// Clone clones the remote into the specified dir.
func Clone(remote, dir string) error {
	output, err := RunGit(filepath.Dir(dir), []string{"clone", remote, filepath.Base(dir)})
	if err != nil {
		return fmt.Errorf(output.Stderr)
	}
	return nil
}
