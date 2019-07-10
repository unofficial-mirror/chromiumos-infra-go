// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
)

// projectBranchName determines the git branch name for the project.
func (c *createBranchRun) projectBranchName(branch string, project repo.Project, original string) string {
	// If the project has only one checkout, then the base branch name is fine.
	var checkouts []string
	manifest := checkout.Manifest()
	for _, proj := range manifest.Projects {
		if proj.Name == project.Name {
			checkouts = append(checkouts, proj.Name)
		}
	}

	if len(checkouts) == 1 {
		return branch
	}

	// Otherwise, the project name needs a suffix. We append its upstream or
	// revision to distinguish it from other checkouts.
	suffix := "-"
	if project.Upstream != "" {
		suffix += git.StripRefs(project.Upstream)
	} else {
		suffix += git.StripRefs(project.Revision)
	}

	// If the revision is itself a branch, we need to strip the old branch name
	// from the suffix to keep naming consistent.
	if original != "" {
		if strings.HasPrefix(suffix, "-"+original+"-") {
			suffix = strings.TrimPrefix(suffix, "-"+original)
		}
	}
	return branch + suffix
}
