// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package checkout

import (
	"gotest.tools/assert"
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/chromiumos/infra/go/internal/git"
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

func TestRunGit_success(t *testing.T) {
	checkout := CrosCheckout{
		root: "",
	}
	project := repo.Project{
		Name: "mock",
		Path: "mock/",
	}

	logMsg := "we currently have pine, oak, and cedar"
	git.CommandRunnerImpl = &cmd.FakeCommandRunnerMulti{
		CommandRunners: []cmd.FakeCommandRunner{
			{
				ExpectedDir: "mock",
				ExpectedCmd: []string{"git", "log"},
				FailCommand: true,
			},
			{
				ExpectedDir: "mock",
				ExpectedCmd: []string{"git", "log"},
				Stdout:      logMsg,
			},
		},
	}

	output, err := checkout.RunGit(project, []string{"log"})
	assert.NilError(t, err)
	assert.Equal(t, output.Stdout, logMsg)
}

func TestRunGit_error(t *testing.T) {
	checkout := CrosCheckout{
		root: "",
	}
	project := repo.Project{
		Name: "mock",
		Path: "mock/",
	}

	git.CommandRunnerImpl = cmd.FakeCommandRunner{
		FailCommand: true,
	}

	_, err := checkout.RunGit(project, []string{"log"})
	assert.ErrorContains(t, err, "failed after 3")
}

// TODO(@jackneus): Finish
