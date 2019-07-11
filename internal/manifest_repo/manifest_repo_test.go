// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package manifest_repo

import (
	"gotest.tools/assert"
	"testing"

	"github.com/golang/mock/gomock"
	mock_checkout "go.chromium.org/chromiumos/infra/go/internal/checkout/mock"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
)

func TestRepairManifest(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	m := mock_checkout.NewMockCheckout(ctl)
	manifestRepo := ManifestRepo{
		checkout: m,
		project: repo.Project{
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
	loadManifestFromFile = func(path string) (map[string]*repo.Manifest, error) {
		return map[string]*repo.Manifest{path: &expectedManifest}, nil
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
