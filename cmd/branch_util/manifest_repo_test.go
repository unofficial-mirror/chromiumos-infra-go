// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
)

func TestRepairManifest_success(t *testing.T) {
	manifestRepo := ManifestRepo{
		Project: repo.Project{
			Name: "chromiumos/manifest",
		},
		ProjectCheckout: "foo",
	}
	branchPathMap := map[string]string{
		"foo/": "branch",
	}
	expectedManifest := repo.Manifest{
		Default: repo.Default{
			RemoteName: "cros",
			Revision:   "test",
		},
		Remotes: []repo.Remote{
			{Name: "cros", Revision: "123"},
			{Name: "remote2", Revision: "124"},
			{Name: "remote3", Revision: "125"},
		},
		Projects: []repo.Project{
			{Name: "chromiumos/foo", Path: "foo/", RemoteName: "cros"},
			{Name: "pinned", Path: "pinned/",
				Annotations: []repo.Annotation{
					{Name: "branch-mode", Value: "pin"},
				},
			},
			{Name: "tot", Path: "tot/",
				Annotations: []repo.Annotation{
					{Name: "branch-mode", Value: "tot"},
				},
			},
		},
	}
	expectedManifest = *expectedManifest.ResolveImplicitLinks()

	workingManifest = expectedManifest
	// Mock out loadManifestFromFile
	loadManifestFromFile = func(path string) (repo.Manifest, error) {
		return expectedManifest, nil
	}
	git.CommandRunnerImpl = cmd.FakeCommandRunner{
		Stdout: "123 test",
	}

	manifest, err := manifestRepo.RepairManifest("dummy_path", branchPathMap)
	assert.NilError(t, err)
	// RepairManifest deletes revision attr on <default>
	assert.Equal(t, manifest.Default.Revision, "")
	// RepairManifest deletes revision attr on <remote>
	for _, remote := range manifest.Remotes {
		assert.Equal(t, remote.Revision, "")
	}
	// RepairManifest properly sets revision on branched projects.
	assert.Equal(t, manifest.Projects[0].Revision, "refs/heads/branch")
	// RepairManifest properly sets revision on pinned projects.
	assert.Equal(t, manifest.Projects[1].Revision, "123")
	// RepairManifest properly sets revision on ToT projects.
	assert.Equal(t, manifest.Projects[2].Revision, "refs/heads/master")
}

func TestRepairManifestsOnDisk(t *testing.T) {
	// Use actual repo implementations
	loadManifestFromFile = repo.LoadManifestFromFile
	loadManifestTree = repo.LoadManifestTree

	defaultManifest := repo.Manifest{
		Includes: []repo.Include{
			{Name: "full.xml"},
		},
	}
	officialManifest := repo.Manifest{
		Includes: []repo.Include{
			{Name: "full.xml"},
		},
	}
	fullManifest := repo.Manifest{
		Default: repo.Default{
			RemoteName: "cros",
			Revision:   "refs/heads/master",
		},
		Projects: []repo.Project{
			{Name: "chromiumos/foo", Path: "foo/"},
		},
		Remotes: []repo.Remote{
			{Name: "cros", Revision: "123"},
		},
	}
	expectedFullManifest := repo.Manifest{
		Default: repo.Default{
			RemoteName: "cros",
		},
		Projects: []repo.Project{
			{Name: "chromiumos/foo",
				Path:       "foo/",
				Revision:   "refs/heads/newbranch",
				RemoteName: "cros"},
		},
		Remotes: []repo.Remote{
			{Name: "cros", Revision: ""},
		},
	}

	tmpDir := "manifestrepotest_tmp_dir"
	tmpDir, err := ioutil.TempDir("", tmpDir)
	defer os.RemoveAll(tmpDir)
	assert.NilError(t, err)

	manifests := make(map[string]*repo.Manifest)
	manifests["default.xml"] = &defaultManifest
	manifests["official.xml"] = &officialManifest
	manifests["full.xml"] = &fullManifest
	manifestPath := make(map[string]string)

	manifestRepo := ManifestRepo{
		Project: repo.Project{
			Name: tmpDir,
		},
		ProjectCheckout: tmpDir,
	}

	git.CommandRunnerImpl = cmd.FakeCommandRunner{
		Stdout: "123 refs/heads/master",
	}

	// Set up
	for manifestName, manifest := range manifests {
		// Write manifest.
		path := filepath.Join(tmpDir, manifestName)
		manifestPath[manifestName] = path
		assert.NilError(t, manifest.Write(path))
	}

	fooProject := fullManifest.Projects[0]
	branchMap := make(map[string]string)
	branchMap[fooProject.Path] = "newbranch"

	err = manifestRepo.RepairManifestsOnDisk(branchMap)
	assert.NilError(t, err)
	// Read repaired manifests from disk, check expectations.
	defaultManifestMap, err := repo.LoadManifestTree(manifestPath["default.xml"])

	assert.NilError(t, err)
	assert.Assert(t,
		reflect.DeepEqual(expectedFullManifest, *defaultManifestMap["full.xml"]))
	assert.Assert(t,
		reflect.DeepEqual(defaultManifest, *defaultManifestMap["default.xml"]))

	officialManifestMap, err := repo.LoadManifestTree(manifestPath["official.xml"])
	assert.NilError(t, err)
	assert.Assert(t,
		reflect.DeepEqual(officialManifest, *officialManifestMap["official.xml"]))
}
