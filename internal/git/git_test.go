// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/assert"
	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/util"
)

func TestRunGit_success(t *testing.T) {
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: "myrepo",
		ExpectedCmd: []string{"git", "log"},
		Stdout:      "success",
	}

	output, err := RunGit("myrepo", []string{"log"})
	assert.NilError(t, err)
	assert.StringsEqual(t, output.Stdout, "success")
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
	assert.StringsEqual(t, output.Stderr, "sudden death")
}

func TestStripRefsHead(t *testing.T) {
	assert.StringsEqual(t, StripRefsHead("refs/heads/foo"), "foo")
	assert.StringsEqual(t, StripRefsHead("foo"), "foo")
}

func TestStripRefs(t *testing.T) {
	assert.StringsEqual(t, StripRefs("refs/remotes/origin/foo"), "foo")
	assert.StringsEqual(t, StripRefs("refs/heads/foo"), "foo")
	assert.StringsEqual(t, StripRefs("foo"), "foo")
}

func TestNormalizeRef(t *testing.T) {
	assert.StringsEqual(t, NormalizeRef("refs/heads/foo"), "refs/heads/foo")
	assert.StringsEqual(t, NormalizeRef("foo"), "refs/heads/foo")
}

func TestIsSHA(t *testing.T) {
	assert.Assert(t, IsSHA("4f4fad584e9c2735af5131f15b697376a3327de5"))
	assert.Assert(t, !IsSHA("refs/heads/master"))
}

func TestGetCurrentBranch_success(t *testing.T) {
	fakeGitRepo := "top-secret-project"
	fakeGitData := "refs/heads/current-branch"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		Stdout:      fakeGitData,
	}
	assert.StringsEqual(t, GetCurrentBranch(fakeGitRepo), "current-branch")
}

func TestGetCurrentBranch_failure(t *testing.T) {
	CommandRunnerImpl = cmd.FakeCommandRunner{
		FailCommand: true,
	}
	assert.StringsEqual(t, GetCurrentBranch("project"), "")
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
	if !reflect.DeepEqual(expectedMatches, branches) {
		t.Fatalf("branches mismatch: got %v, expected %v", branches, expectedMatches)
	}

	expectedMatches = []string{"refs/heads/foo"}
	branches, err = MatchBranchName(fakeGitRepo, regexp.MustCompile("Foo$"))
	assert.NilError(t, err)
	if !reflect.DeepEqual(expectedMatches, branches) {
		t.Fatalf("branches mismatch: got %v, expected %v", branches, expectedMatches)
	}
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
	if !reflect.DeepEqual(expectedMatches, branches) {
		t.Fatalf("branches mismatch: got %v, expected %v", branches, expectedMatches)
	}
}

func TestGetRepoRevision(t *testing.T) {
	sha := "6446dfef4b55689046395c2db7ba7c35377927fe"
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "rev-parse", "HEAD"},
		ExpectedDir: "project",
		Stdout:      sha,
	}
	res, err := GetGitRepoRevision("project", "")
	assert.NilError(t, err)
	assert.StringsEqual(t, res, sha)
}

func TestIsReachable_true(t *testing.T) {
	fakeGitRepo := "gitRepo"
	toRef := "beef"
	fromRef := "deaf"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "merge-base", "--is-ancestor", toRef, fromRef},
		ExpectedDir: fakeGitRepo,
	}

	ok, err := IsReachable(fakeGitRepo, toRef, fromRef)
	assert.NilError(t, err)
	assert.Assert(t, ok)
}

func TestIsReachable_false(t *testing.T) {
	fakeGitRepo := "gitRepo"
	toRef := "beef"
	fromRef := "deaf"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedCmd: []string{"git", "merge-base", "--is-ancestor", toRef, fromRef},
		ExpectedDir: fakeGitRepo,
		FailCommand: true,
		FailError:   "exit status 1",
	}

	ok, err := IsReachable(fakeGitRepo, toRef, fromRef)
	assert.NilError(t, err)
	assert.Assert(t, !ok)
}

func TestIsReachable_self(t *testing.T) {
	CommandRunnerImpl = cmd.RealCommandRunner{}

	tmpDir := "gittest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)

	assert.NilError(t, err)
	// Create repo.
	assert.NilError(t, Init(tmpDir, false))
	// Make commit.
	commit, err := CommitEmpty(tmpDir, "empty commit")
	assert.NilError(t, err)
	// Check that a SHA is reachable from itself.
	ok, err := IsReachable(tmpDir, commit, commit)
	assert.NilError(t, err)
	assert.Assert(t, ok)
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
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "rev-parse", "HEAD"},
				Stdout:      "abcde12345\n\n\t\n",
			},
		},
	}

	commit, err := CommitAll(fakeGitRepo, commitMsg)
	assert.NilError(t, err)
	assert.StringsEqual(t, commit, "abcde12345")
}

func TestCommitEmpty(t *testing.T) {
	fakeGitRepo := "repo"
	commitMsg := "commit"

	CommandRunnerImpl = &cmd.FakeCommandRunnerMulti{
		CommandRunners: []cmd.FakeCommandRunner{
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "commit", "-m", commitMsg, "--allow-empty"},
			},
			{
				ExpectedDir: fakeGitRepo,
				ExpectedCmd: []string{"git", "rev-parse", "HEAD"},
				Stdout:      "abcde12345\n\n\t\n",
			},
		},
	}

	commit, err := CommitEmpty(fakeGitRepo, commitMsg)
	assert.NilError(t, err)
	assert.StringsEqual(t, commit, "abcde12345")
}

func TestPushRef(t *testing.T) {
	fakeGitRepo := "repo"
	localRef := "commitId"

	remoteRef := RemoteRef{
		Remote: "remote",
		Ref:    "ref",
	}

	pushStr := fmt.Sprintf("%s:%s", localRef, remoteRef.Ref)
	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: fakeGitRepo,
		ExpectedCmd: []string{"git", "push", remoteRef.Remote, pushStr, "--dry-run", "--force"},
	}

	opts := GitOpts{
		DryRun: true,
		Force:  true,
	}
	err := PushRef(fakeGitRepo, localRef, remoteRef, opts)
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
	_, err = CommitAll(tmpDir, "init commit")
	assert.NilError(t, err)
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
	_, err = CommitAll(tmpDir, "init commit")
	assert.NilError(t, err)
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
	_, err = CommitAll(tmpDir, "init commit")
	assert.NilError(t, err)
	// Create test branch.
	assert.NilError(t, CreateBranch(tmpDir, branchName))
	// Make a change to branch.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(tmpDir, "bar"), []byte("bar"), 0644))
	_, err = CommitAll(tmpDir, "init commit")
	assert.NilError(t, err)
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

func TestRemoteBranches(t *testing.T) {
	CommandRunnerImpl = cmd.RealCommandRunner{}

	local, err := ioutil.TempDir("", "gittest_tmp_dir")
	defer os.RemoveAll(local)

	remote, err := ioutil.TempDir("", "gittest_tmp_dir")
	defer os.RemoveAll(remote)

	assert.NilError(t, err)
	// Create repos.
	assert.NilError(t, Init(local, false))
	assert.NilError(t, Init(remote, false))
	// Create local master branch.
	assert.NilError(t, CreateBranch(local, "master"))
	// In order for the ref to be created, need to commit something.
	assert.NilError(t, ioutil.WriteFile(filepath.Join(local, "foo"), []byte("foo"), 0644))
	_, err = CommitAll(local, "init commit")
	assert.NilError(t, err)
	// Add remote.
	assert.NilError(t, AddRemote(local, "remote", remote))
	// Create remote branches.
	remoteRef := RemoteRef{
		Remote: "remote",
		Ref:    "foo",
	}
	opts := GitOpts{
		DryRun: false,
		Force:  true,
	}
	assert.NilError(t, PushRef(local, "HEAD", remoteRef, opts))
	remoteRef.Ref = "bar"
	assert.NilError(t, PushRef(local, "HEAD", remoteRef, opts))

	branches, err := RemoteBranches(local, "remote")
	assert.NilError(t, err)
	assert.Assert(t, util.UnorderedEqual(branches, []string{"foo", "bar"}))
	ok, err := RemoteHasBranch(local, "remote", "foo")
	assert.NilError(t, err)
	assert.Assert(t, ok)
	ok, err = RemoteHasBranch(local, "remote", "baz")
	assert.NilError(t, err)
	assert.Assert(t, !ok)
}

func TestResolveRemoteSymbolicRef(t *testing.T) {
	remote := "remote"

	CommandRunnerImpl = cmd.FakeCommandRunner{
		ExpectedDir: "foo",
		ExpectedCmd: []string{"git", "ls-remote", "-q", "--symref", "--exit-code", remote, "HEAD"},
		Stdout:      "ref: refs/heads/main\tHEAD\n5f6803b100bb3cd0f534e96e88c91373e8ed1c44\tHEAD\n",
	}
	ref, err := ResolveRemoteSymbolicRef("foo", remote, "HEAD")
	assert.NilError(t, err)
	assert.StringsEqual(t, ref, "refs/heads/main")
}
