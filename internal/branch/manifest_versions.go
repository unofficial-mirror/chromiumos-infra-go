// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package branch

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

// var and not const for testing purposes
var (
	manifestVersionsRemote = "https://chrome-internal.googlesource.com/chromeos/manifest-versions"
)

// GetWorkingManifestForVersion gets the manifest in manifest-versions/buildspecs for the
// given version string (of the format %d.%d.%d).
func GetWorkingManifestForVersion(version string) (*repo.Manifest, error) {
	tmpDir, err := ioutil.TempDir("", "tmp-manifest-version-repo")
	defer os.RemoveAll(tmpDir)
	if err != nil {
		return nil, err
	}

	errs := []error{
		git.Init(tmpDir, false),
		git.AddRemote(tmpDir, "origin", manifestVersionsRemote),
		git.RunGitIgnoreOutput(tmpDir, []string{"fetch", "origin", "master", "--depth", "1"}),
	}
	for _, err := range errs {
		if err != nil {
			return nil, errors.Annotate(err, "failed to fetch manifest-versions/master").Err()
		}
	}
	// To avoid checking out all of buildspecs/, which is multiple Gb, we ls-tree the remote branch to get a list
	// of files. We then find the file we want and only checkout that file.
	// We still have to do a fetch for the full branch, though, which is expensive (even with --depth 1).
	output, err := git.RunGit(tmpDir, []string{"ls-tree", "origin/master", "-r", "--name-only"})
	if err != nil {
		return nil, errors.Annotate(err, "failed to list files in manifest-versions/master").Err()
	}
	manifestName := fmt.Sprintf("%s.xml", version)
	manifestPath := ""
	for _, line := range strings.Split(output.Stdout, "\n") {
		if line == "" {
			continue
		}
		// i.e. if the line is buildspecs**<version>.xml
		if strings.HasPrefix(line, "buildspecs/") && strings.HasSuffix(line, "/"+manifestName) {
			if manifestPath != "" {
				return nil, fmt.Errorf("multiple manifests with name %s exist in manifest-versions", manifestName)
			}
			manifestPath = line
		}
	}
	if manifestPath == "" {
		return nil, fmt.Errorf("manifest %s does not exist in manifest-versions", manifestName)
	}

	if err := git.RunGitIgnoreOutput(tmpDir, []string{"checkout", "origin/master", "--", manifestPath}); err != nil {
		return nil, errors.Annotate(err, "failed to checkout %s", manifestPath).Err()
	}
	manifestPath = filepath.Join(tmpDir, manifestPath)

	return repo.LoadManifestFromFileWithIncludes(manifestPath)
}
