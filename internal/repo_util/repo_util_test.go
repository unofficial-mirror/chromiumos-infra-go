package repo_util

import (
	"bytes"
	"context"
	"fmt"
	"gotest.tools/assert"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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

func TestInitialize_success(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	manifestUrl := "https://chromium.googlesource.com/chromiumos/manifest/foo.xml"
	commandRunnerImpl = fakeCommandRunner{
		expectedCmd: []string{"repo", "init", "--manifest-url", manifestUrl},
		expectedDir: tmpDir,
	}
	repo, err := Initialize(tmpDir, manifestUrl, "repo")
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, repo.root, tmpDir)
}

func TestInitialize_repoexists(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	// .repo directory already exists
	assert.NilError(t, os.Mkdir(filepath.Join(tmpDir, ".repo"), 0775))

	manifestUrl := "https://chromium.googlesource.com/chromiumos/manifest/foo.xml"
	_, err = Initialize(tmpDir, manifestUrl, "repo")
	assert.ErrorContains(t, err, "existing")
}

func TestInitialize_failure(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	manifestUrl := "https://chromium.googlesource.com/chromiumos/manifest/foo.xml"
	commandRunnerImpl = fakeCommandRunner{
		failCommand: true,
	}
	_, err = Initialize(tmpDir, manifestUrl, "repo")
	assert.ErrorContains(t, err, "")
}

func TestSyncToFile_success(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	// Create .repo folder
	assert.NilError(t, os.Mkdir(filepath.Join(tmpDir, ".repo"), 0775))
	// Create manifest file
	file, err := ioutil.TempFile(tmpDir, "foo*.xml")
	assert.NilError(t, err)

	manifestFile := file.Name()
	commandRunnerImpl = fakeCommandRunner{
		expectedCmd: []string{"repo", "sync", "--manifest-name", manifestFile},
	}
	err = SyncToFile(tmpDir, manifestFile, "repo")
	assert.NilError(t, err)
}

func TestSyncToFile_manifest_missing(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	// Create .repo folder
	assert.NilError(t, os.Mkdir(filepath.Join(tmpDir, ".repo"), 0775))

	commandRunnerImpl = fakeCommandRunner{}
	err = SyncToFile(tmpDir, "foo", "repo")
	assert.ErrorContains(t, err, "exist")
}

func TestSyncToFile_repo_no_init(t *testing.T) {
	tmpDir := "repotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	commandRunnerImpl = fakeCommandRunner{}
	err = SyncToFile(tmpDir, "foo", "repo")
	assert.ErrorContains(t, err, "init")
}
