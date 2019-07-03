// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"bytes"
	"context"
	"fmt"
	"gotest.tools/assert"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	stdout      string
	stderr      string
	expectedCmd []string
	expectedDir string
	failCommand bool
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c fakeCommandRunner) runCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error {
	stdoutBuf.WriteString(c.stdout)
	stderrBuf.WriteString(c.stderr)
	cmd := append([]string{name}, args...)
	if len(c.expectedCmd) > 0 {
		if !equal(cmd, c.expectedCmd) {
			expectedCmd := strings.Join(c.expectedCmd, " ")
			actualCmd := strings.Join(cmd, " ")
			return fmt.Errorf("wrong cmd; expected %s got %s", expectedCmd, actualCmd)
		}
	}
	if c.expectedDir != "" {
		if dir != c.expectedDir {
			return fmt.Errorf("wrong cmd dir; expected %s got %s", c.expectedDir, dir)
		}
	}
	if c.failCommand {
		return &exec.ExitError{}
	}
	return nil
}

func TestStripRefsHead(t *testing.T) {
	assert.Equal(t, StripRefsHead("refs/heads/foo"), "foo")
	assert.Equal(t, StripRefsHead("foo"), "foo")
}

func TestStripRefs(t *testing.T) {
	assert.Equal(t, StripRefs("refs/remotes/origin/foo"), "foo")
	assert.Equal(t, StripRefs("refs/heads/foo"), "foo")
	assert.Equal(t, StripRefs("foo"), "foo")
}

func TestNormalizeRef(t *testing.T) {
	assert.Equal(t, NormalizeRef("refs/heads/foo"), "refs/heads/foo")
	assert.Equal(t, NormalizeRef("foo"), "refs/heads/foo")
}

func TestGetCurrentBranch_success(t *testing.T) {
	fakeGitRepo := "top-secret-project"
	fakeGitData := "refs/heads/current-branch"

	commandRunnerImpl = fakeCommandRunner{
		expectedDir: fakeGitRepo,
		stdout:      fakeGitData,
	}
	assert.Equal(t, GetCurrentBranch(fakeGitRepo), "current-branch")
}

func TestGetCurrentBranch_failure(t *testing.T) {
	commandRunnerImpl = fakeCommandRunner{
		failCommand: true,
	}
	assert.Equal(t, GetCurrentBranch("project"), "")
}

func TestMatchBranchName_success(t *testing.T) {
	fakeGitRepo := "top-secret-project"
	fakeGitData := "e9cb56bd9af9365b43f82cecf28cc76d49df1f72  refs/heads/foo\n" +
		"f9c1bb630f4475058d4a9db4aea52fc89d8f7b0d  refs/heads/bar\n" +
		"2102915989de21d9251c11f0a7b5307e175e7677  refs/heads/foobar\n" +
		"04975f9439ff75502b33d9491155692736e05b07  refs/heads/baz\n"

	commandRunnerImpl = fakeCommandRunner{
		expectedCmd: []string{"git", "ls-remote", fakeGitRepo},
		expectedDir: fakeGitRepo,
		stdout:      fakeGitData,
	}

	expectedMatches := []string{"refs/heads/foo", "refs/heads/foobar"}
	branches, err := MatchBranchName(fakeGitRepo, regexp.MustCompile("Foo"))
	assert.NilError(t, err)
	assert.DeepEqual(t, expectedMatches, branches)

	expectedMatches = []string{"refs/heads/foo"}
	branches, err = MatchBranchName(fakeGitRepo, regexp.MustCompile("Foo$"))
	assert.NilError(t, err)
	assert.DeepEqual(t, expectedMatches, branches)
}

func TestGetRepoRevision(t *testing.T) {
	sha := "6446dfef4b55689046395c2db7ba7c35377927fe"
	commandRunnerImpl = fakeCommandRunner{
		expectedCmd: []string{"git", "rev-parse", "HEAD"},
		expectedDir: "project",
		stdout:      sha,
	}
	res, err := GetGitRepoRevision("project")
	assert.NilError(t, err)
	assert.Equal(t, res, sha)
}
