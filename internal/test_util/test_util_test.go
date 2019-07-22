// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test_util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	// "go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"gotest.tools/assert"
)

// createFooBarBaz creates foo bar baz file structure, the greatest file structure on earth
func createFooBarBaz(t *testing.T, root, bazContents string) {
	assert.NilError(t, os.Mkdir(filepath.Join(root, "foo"), 0755))
	assert.NilError(t, os.Mkdir(filepath.Join(root, "foo", "bar"), 0755))
	assert.NilError(t, ioutil.WriteFile(filepath.Join(root, "foo", "bar", "baz"), []byte(bazContents), 0666))
}

// checkFooBarBaz checks the foo bar baz file structure, the greatest file structure on earth
func checkFooBarBaz(t *testing.T, root, bazContents string) {
	_, err := os.Stat(filepath.Join(root, "foo"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(root, "foo", "bar"))
	assert.NilError(t, err)
	snapshotBazPath := filepath.Join(root, "foo", "bar", "baz")
	_, err = os.Stat(snapshotBazPath)
	assert.NilError(t, err)
	// Check contents of bar/baz.
	contents, err := ioutil.ReadFile(snapshotBazPath)
	assert.NilError(t, err)
	assert.Equal(t, string(contents), bazContents)
}

func TestUnorderedEqual(t *testing.T) {
	a := []string{"a", "b", "c", "a"}
	b := []string{"b", "c", "a", "a"}
	c := []string{"a", "b", "b", "c"}
	assert.Assert(t, UnorderedEqual(a, b))
	assert.Assert(t, !UnorderedEqual(a, c))
}

func TestUnorderedContains(t *testing.T) {
	a := []string{"a", "b", "c", "a"}
	b := []string{"b", "c"}
	c := []string{"b", "d"}
	assert.Assert(t, UnorderedContains(a, b))
	assert.Assert(t, !UnorderedContains(a, c))
}

func TestAssertContentsEqual(t *testing.T) {
	foo1, err := ioutil.TempDir("", "nodiff_test")
	assert.NilError(t, err)
	defer os.RemoveAll(foo1)
	createFooBarBaz(t, foo1, "ヽ༼ຈل͜ຈ༽ﾉ")

	foo2, err := ioutil.TempDir("", "nodiff_test")
	assert.NilError(t, err)
	defer os.RemoveAll(foo2)
	createFooBarBaz(t, foo2, "ヽ༼ຈل͜ຈ༽ﾉ")

	foo3, err := ioutil.TempDir("", "nodiff_test")
	assert.NilError(t, err)
	defer os.RemoveAll(foo3)
	createFooBarBaz(t, foo3, "ヽ༼ಠل͜ಠ༽ﾉ")

	assert.NilError(t, AssertContentsEqual(foo1, foo2))
	assert.ErrorContains(t, AssertContentsEqual(foo1, foo3), "differ")
}

func TestAssertContentsEqual_files(t *testing.T) {
	foo, err := ioutil.TempDir("", "nodiff_test")
	assert.NilError(t, err)
	defer os.RemoveAll(foo)
	same1 := filepath.Join(foo, "same1")
	same2 := filepath.Join(foo, "same2")
	different := filepath.Join(foo, "different")

	assert.NilError(t, ioutil.WriteFile(same1, []byte("same"), 0644))
	assert.NilError(t, ioutil.WriteFile(same2, []byte("same"), 0644))
	assert.NilError(t, ioutil.WriteFile(different, []byte("different"), 0644))

	assert.NilError(t, AssertContentsEqual(same1, same2))
	assert.ErrorContains(t, AssertContentsEqual(same1, different), "files differ")
}

func TestAssertContentsEqual_typeMismatch(t *testing.T) {
	foo1, err := ioutil.TempDir("", "nodiff_test")
	assert.NilError(t, err)
	defer os.RemoveAll(foo1)

	foo2, err := ioutil.TempDir("", "nodiff_test")
	assert.NilError(t, err)
	defer os.RemoveAll(foo2)
	filePath := filepath.Join(foo2, "foo")
	assert.NilError(t, ioutil.WriteFile(filePath, []byte("file contents"), 0644))

	assert.ErrorContains(t, AssertContentsEqual(foo1, filePath), "is a dir")
}

func TestAssertGitBranches_success(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "assert_git_branches_test")
	assert.NilError(t, err)
	defer os.RemoveAll(tmpDir)

	assert.NilError(t, git.Init(tmpDir, false))

	branches := []string{"branch1", "branch2", "branch3"}
	for _, branch := range branches {
		assert.NilError(t, git.CreateBranch(tmpDir, branch))
		// Empty commit so that branch is not "unborn".
		_, err := git.RunGit(tmpDir, []string{"commit", "-m", "init", "--allow-empty"})
		assert.NilError(t, err)
	}

	assert.NilError(t, AssertGitBranches(tmpDir, branches))
}

func TestAssertGitBranches_failure(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "assert_git_branches_test")
	assert.NilError(t, err)
	defer os.RemoveAll(tmpDir)

	assert.NilError(t, git.Init(tmpDir, false))
	assert.ErrorContains(t, AssertGitBranches(tmpDir, []string{"master", "foo"}), "mismatch")
}
