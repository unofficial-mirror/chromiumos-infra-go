// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package branch

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"gotest.tools/assert"
)

func TestGetWorkingManifestForVersion(t *testing.T) {
	tmpRepo, err := ioutil.TempDir("", "")
	assert.NilError(t, err)
	defer os.RemoveAll(tmpRepo)

	assert.NilError(t, git.Init(tmpRepo, false))

	// Make some milestone subdirectories.
	milestonePath := filepath.Join(tmpRepo, "buildspecs", "12")
	assert.NilError(t, os.MkdirAll(milestonePath, 0777))

	manifestFileContents := []byte("<manifest><project name=\"foo\" path=\"bar\"/></manifest>")
	parsedManifest := repo.Manifest{
		Projects: []repo.Project{
			{Name: "foo", Path: "bar"},
		},
		Includes: []repo.Include{},
	}

	assert.NilError(t,
		ioutil.WriteFile(filepath.Join(milestonePath, "12345.0.0.xml"), manifestFileContents, 0666))

	// Duplicate manifest version -- bad!
	milestonePath = filepath.Join(tmpRepo, "buildspecs", "13")
	assert.NilError(t, os.MkdirAll(milestonePath, 0777))
	assert.NilError(t, ioutil.WriteFile(filepath.Join(milestonePath, "12345.1.0.xml"), manifestFileContents, 0666))
	milestonePath = filepath.Join(tmpRepo, "buildspecs", "14")
	assert.NilError(t, os.MkdirAll(milestonePath, 0777))
	assert.NilError(t, ioutil.WriteFile(filepath.Join(milestonePath, "12345.1.0.xml"), manifestFileContents, 0666))

	_, err = git.CommitAll(tmpRepo, "my buildspecs bring all the boys to the yard")
	assert.NilError(t, err)

	manifestVersionsRemote = tmpRepo
	// Successful -- exactly one manifest exists.
	manifest, err := GetWorkingManifestForVersion("12345.0.0")
	assert.NilError(t, err)
	assert.Assert(t, reflect.DeepEqual(*manifest, parsedManifest))
	// Error -- more than one manifest exists.
	_, err = GetWorkingManifestForVersion("12345.1.0")
	assert.ErrorContains(t, err, "multiple manifests")
	// Error -- no manifest exists.
	_, err = GetWorkingManifestForVersion("12345.2.0")
	assert.ErrorContains(t, err, "does not exist")
}
