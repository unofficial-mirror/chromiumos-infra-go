// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package manifest_repo

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	mock_checkout "go.chromium.org/chromiumos/infra/go/internal/checkout/mock"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
)

func TestRepairManifest(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	manifestRepo := ManifestRepo{
		Checkout: m,
		Project: repo.Project{
			Name: "chromiumos/manifest",
		},
	}
	branchPathMap := map[string]string{
		"foo/": "branch",
	}
	expectedManifest := repo.Manifest{
		Default: repo.Default{
			Revision: "test",
		},
		Remotes: []repo.Remote{
			{Name: "remote1", Revision: "123"},
			{Name: "remote2", Revision: "124"},
			{Name: "remote3", Revision: "125"},
		},
		Projects: []repo.Project{
			{Name: "foo", Path: "foo/"},
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

	// Mock out loadManifestFromFile
	loadManifestFromFile = func(path string) (repo.Manifest, error) {
		return expectedManifest, nil
	}
	for _, project := range expectedManifest.Projects {
		m.EXPECT().
			EnsureProject(project).
			Return(nil)
	}
	m.EXPECT().
		GitRevision(expectedManifest.Projects[1]).
		Return("123", nil)

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
		Projects: []repo.Project{
			{Name: "foo", Path: "foo/", Revision: "123"},
		},
		Remotes: []repo.Remote{
			{Name: "cros", Revision: "123"},
		},
	}
	expectedFullManifest := repo.Manifest{
		Projects: []repo.Project{
			{Name: "foo", Path: "foo/", Revision: "refs/heads/newbranch"},
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

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	m := mock_checkout.NewMockCheckout(ctl)
	manifestRepo := ManifestRepo{
		Checkout: m,
		Project: repo.Project{
			Name: tmpDir,
		},
	}

	// Set up
	for manifestName, manifest := range manifests {
		// Write manifest.
		path := filepath.Join(tmpDir, manifestName)
		manifestPath[manifestName] = path
		assert.NilError(t, manifest.Write(path))
		// Mock expectations.
		m.EXPECT().
			AbsoluteProjectPath(manifestRepo.Project, manifestName).
			Return(path).
			AnyTimes()
	}

	fooProject := fullManifest.Projects[0]
	branchMap := make(map[string]string)
	branchMap[fooProject.Path] = "newbranch"
	m.EXPECT().
		EnsureProject(fooProject).
		Return(nil)

	err = manifestRepo.RepairManifestsOnDisk(branchMap)

	// Read repaired manifests from disk, check expectations.
	defaultManifestMap, err := repo.LoadManifestTree(manifestPath["default.xml"])
	assert.NilError(t, err)
	assert.Assert(t,
		reflect.DeepEqual(expectedFullManifest, *defaultManifestMap[manifestPath["full.xml"]]))
	assert.Assert(t,
		reflect.DeepEqual(defaultManifest, *defaultManifestMap[manifestPath["default.xml"]]))

	officialManifestMap, err := repo.LoadManifestTree(manifestPath["official.xml"])
	assert.NilError(t, err)
	assert.Assert(t,
		reflect.DeepEqual(officialManifest, *officialManifestMap[manifestPath["official.xml"]]))
}
