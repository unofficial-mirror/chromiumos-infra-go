// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
)

func TestProjectFetchUrl(t *testing.T) {
	workingManifest = repo.Manifest{
		Remotes: []repo.Remote{
			{Name: "remote", Fetch: "file:///tmp/path/to/remote"},
		},
		Projects: []repo.Project{
			{Path: "foo/bar/project", Name: "foo/bar/project", RemoteName: "remote"},
		},
	}
	url, err := projectFetchUrl("foo/bar/project")
	assert.NilError(t, err)
	assert.Equal(t, url, "file:///tmp/path/to/remote/foo/bar/project")
}
