// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"fmt"
	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"gotest.tools/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestRunGit_success(t *testing.T) {
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: "myrepo",
		ExpectedCmd: []string{"git", "log"},
		Stdout:      "success",
	}

	output, err := RunGit("myrepo", []string{"log"})
	assert.NilError(t, err)
	assert.Equal(t, output.Stdout, "success")
}

func TestRunGit_error(t *testing.T) {
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: "myrepo",
		ExpectedCmd: []string{"git", "log"},
		Stdout:      "I don't feel so go--",
		Stderr:      "sudden death",
		FailCommand: true,
	}

	output, err := RunGit("myrepo", []string{"log"})
	assert.Assert(t, err != nil)
	assert.Equal(t, output.Stderr, "sudden death")
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

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		Stdout:      fakeGitData,
	}
	assert.Equal(t, GetCurrentBranch(fakeGitRepo), "current-branch")
}

func TestGetCurrentBranch_failure(t *testing.T) {
	CommandRunnerImpl = cmd.FakeCommandRunner{
		FailCommand: true,
	}
	assert.Equal(t, GetCurrentBranch("project"), "")
}

func TestMatchBranchName_success(t *testing.T) {
	fakeGitRepo := "top-secret-project"
	fakeGitData := "e9cb56bd9af9365b43f82cecf28cc76d49df1f72  refs/heads/foo\n" +
		"f9c1bb630f4475058d4a9db4aea52fc89d8f7b0d  refs/heads/bar\n" +
		"2102915989de21d9251c11f0a7b5307e175e7677  refs/heads/foobar\n" +
		"04975f9439ff75502b33d9491155692736e05b07  refs/heads/baz\n"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "show-ref"},
		ExpectedDir: fakeGitRepo,
		Stdout:      fakeGitData,
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

func TestMatchBranchNameWithNamespace_success(t *testing.T) {
	fakeGitRepo := "top-secret-project"
	fakeGitData := "e9cb56bd9af9365b43f82cecf28cc76d49df1f72  refs/changes/foo\n" +
		"f9c1bb630f4475058d4a9db4aea52fc89d8f7b0d  refs/changes/bar\n" +
		"2102915989de21d9251c11f0a7b5307e175e7677  refs/heads/foobar\n" +
		"04975f9439ff75502b33d9491155692736e05b07  refs/heads/baz\n"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "show-ref"},
		ExpectedDir: fakeGitRepo,
		Stdout:      fakeGitData,
	}

	expectedMatches := []string{"foobar"}
	pattern := regexp.MustCompile("FOO")
	namespace := regexp.MustCompile("refs/heads/")
	branches, err := MatchBranchNameWithNamespace(fakeGitRepo, pattern, namespace)
	assert.NilError(t, err)
	assert.DeepEqual(t, expectedMatches, branches)
}

func TestGetRepoRevision(t *testing.T) {
	sha := "6446dfef4b55689046395c2db7ba7c35377927fe"
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "rev-parse", "HEAD"},
		ExpectedDir: "project",
		Stdout:      sha,
	}
	res, err := GetGitRepoRevision("project")
	assert.NilError(t, err)
	assert.Equal(t, res, sha)
}

func TestCreateBranch(t *testing.T) {
	fakeGitRepo := "top-secret-project"
	branchName := "project z"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "checkout", "-B", branchName},
	}
	assert.NilError(t, CreateBranch(fakeGitRepo, branchName))
}

func TestCreateTrackingBranch(t *testing.T) {
	fakeGitRepo := "repo"
	branchName := "branch"
	remoteRef := RemoteRef{
		Remote: "remote",
		Ref:    "master",
	}
	refspec := fmt.Sprintf("%s/%s", remoteRef.Remote, remoteRef.Ref)

	CommandRunnerImpl = &cmd.FakeCommandRunnerMulti{
		CommandRunners: []cmd.FakeCommandRunner{
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "fetch", remoteRef.Remote, remoteRef.Ref},
			},
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "checkout", "-b", branchName, "-t", refspec},
			},
		},
	}

	err := CreateTrackingBranch(fakeGitRepo, branchName, remoteRef)
	assert.NilError(t, err)
}

func TestCommitAll(t *testing.T) {
	fakeGitRepo := "repo"
	commitMsg := "commit"

	CommandRunnerImpl = &cmd.FakeCommandRunnerMulti{
		CommandRunners: []cmd.FakeCommandRunner{
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "add", "-A"},
			},
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "commit", "-m", commitMsg},
			},
		},
	}

	err := CommitAll(fakeGitRepo, commitMsg)
	assert.NilError(t, err)
}

func TestCommitEmpty(t *testing.T) {
	fakeGitRepo := "repo"
	commitMsg := "commit"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "commit", "-m", commitMsg, "--allow-empty"},
	}

	err := CommitEmpty(fakeGitRepo, commitMsg)
	assert.NilError(t, err)
}

func TestPushChanges(t *testing.T) {
	fakeGitRepo := "da-bank"
	commitMsg := "da-money"
	localRef := "da-vault"

	remoteRef := RemoteRef{
		Remote: "da-family",
		Ref:    "da-van",
	}

	pushStr := fmt.Sprintf("%s:%s", localRef, remoteRef.Ref)
	CommandRunnerImpl = &cmd.FakeCommandRunnerMulti{
		CommandRunners: []cmd.FakeCommandRunner{
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "add", "-A"},
			},
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "commit", "-m", commitMsg},
			},
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "push", remoteRef.Remote, pushStr, "--dry-run"},
			},
		},
	}

	err := PushChanges(fakeGitRepo, localRef, commitMsg, true, remoteRef)
	assert.NilError(t, err)
}

func TestPush(t *testing.T) {
	fakeGitRepo := "repo"
	localRef := "commitId"

	remoteRef := RemoteRef{
		Remote: "remote",
		Ref:    "ref",
	}

	pushStr := fmt.Sprintf("%s:%s", localRef, remoteRef.Ref)
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "push", remoteRef.Remote, pushStr, "--dry-run"},
	}

	err := Push(fakeGitRepo, localRef, true, remoteRef)
	assert.NilError(t, err)
}

func TestInit(t *testing.T) {
	fakeGitRepo := "top-secret-project"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "init"},
	}
	assert.NilError(t, Init(fakeGitRepo, false))
}

func TestInit_bare(t *testing.T) {
	fakeGitRepo := "top-secret-project"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "init", "--bare"},
	}
	assert.NilError(t, Init(fakeGitRepo, true))
}

func TestAddRemote(t *testing.T) {
	fakeGitRepo := "repo"
	remoteName := "remote"
	remoteLoc := "remote/"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "remote", "add", remoteName, remoteLoc},
	}
	assert.NilError(t, AddRemote(fakeGitRepo, remoteName, remoteLoc))
}

func TestCheckout(t *testing.T) {
	CommandRunnerImpl = cmd.RealCommandRunner{}

	tmpDir := "gittest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)

	assert.NilError(t, err)
	// Create repo.
	assert.NilError(t, Init(tmpDir, false))
	// Create first branch.
	assert.NilError(t, CreateBranch(tmpDir, "branch1"))
	// In order for the ref to be created, need to commit something.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(tmpDir, "foo"), []byte("foo"), 0644))
	assert.NilError(t, CommitAll(tmpDir, "init commit"))
	// Create second branch (will switch to this branch).
	assert.NilError(t, CreateBranch(tmpDir, "branch2"))
	// Try checking out a nonexistent branch.
	assert.ErrorContains(t, Checkout(tmpDir, "branch3"), "did not match any")
	// Try checking out the first branch.
	assert.NilError(t, Checkout(tmpDir, "branch1"))
}

func TestDeleteBranch_success(t *testing.T) {
	CommandRunnerImpl = cmd.RealCommandRunner{}

	tmpDir := "gittest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	branchName := "newbranch"

	assert.NilError(t, err)
	// Create repo.
	assert.NilError(t, Init(tmpDir, false))
	// Create master branch.
	assert.NilError(t, CreateBranch(tmpDir, "master"))
	// In order for the ref to be created, need to commit something.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(tmpDir, "foo"), []byte("foo"), 0644))
	assert.NilError(t, CommitAll(tmpDir, "init commit"))
	// Create branch to be deleted.
	assert.NilError(t, CreateBranch(tmpDir, branchName))
	// Switch back to master.
	assert.NilError(t, Checkout(tmpDir, "master"))
	err = DeleteBranch(tmpDir, branchName, true)
	assert.NilError(t, err)
}

func TestDeleteBranch_inBranch(t *testing.T) {
	CommandRunnerImpl = cmd.RealCommandRunner{}

	tmpDir := "gittest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	branchName := "newbranch"

	assert.NilError(t, err)
	// Create repo.
	assert.NilError(t, Init(tmpDir, false))
	// Create branch.
	assert.NilError(t, CreateBranch(tmpDir, branchName))
	err = DeleteBranch(tmpDir, branchName, true)
	assert.ErrorContains(t, err, "checked out")
}

func TestDeleteBranch_unmerged(t *testing.T) {
	CommandRunnerImpl = cmd.RealCommandRunner{}

	tmpDir := "gittest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	branchName := "newbranch"

	assert.NilError(t, err)
	// Create repo.
	assert.NilError(t, Init(tmpDir, false))
	// Create master branch.
	assert.NilError(t, CreateBranch(tmpDir, "master"))
	// In order for the ref to be created, need to commit something.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(tmpDir, "foo"), []byte("foo"), 0644))
	assert.NilError(t, CommitAll(tmpDir, "init commit"))
	// Create test branch.
	assert.NilError(t, CreateBranch(tmpDir, branchName))
	// Make a change to branch.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(tmpDir, "bar"), []byte("bar"), 0644))
	assert.NilError(t, CommitAll(tmpDir, "init commit"))
	// Switch back to master.
	assert.NilError(t, Checkout(tmpDir, "master"))
	// Should not be able to delete.
	assert.ErrorContains(t, DeleteBranch(tmpDir, branchName, false), "fully merged")
}

func TestClone(t *testing.T) {
	dest := "foo/bar"
	remote := "remote"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: "foo",
		ExpectedCmd: []string{"git", "clone", remote, "bar"},
	}
	assert.NilError(t, Clone(remote, dest))
}
