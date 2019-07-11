// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package checkout

import (
	"gotest.tools/assert"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/repo"
)

func TestEnsureProject(t *testing.T) {
	checkout := CrosCheckout{
		root: "",
	}
	project := repo.Project{
		Name: "mock",
		Path: "mock/",
	}
	err := checkout.EnsureProject(project)
	assert.NilError(t, err)

	project = repo.Project{
		Name: "imaginary",
		Path: "imaginary/",
	}
	err = checkout.EnsureProject(project)
	assert.ErrorContains(t, err, "does not exist")
}

func TestGitRevision(t *testing.T) {
	// TODO(@jackneus): Figure out how to mock git module.
	// Not super critical because GitRevision is a one-line wrapper.
}

// TODO(@jackneus): Finish
