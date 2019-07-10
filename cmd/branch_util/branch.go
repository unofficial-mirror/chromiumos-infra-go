// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
)

var (
	GOB_HOST              = "%s.googlesource.com"
	EXTERNAL_GOB_INSTANCE = "chromium"
	EXTERNAL_GOB_HOST     = fmt.Sprintf(GOB_HOST, EXTERNAL_GOB_INSTANCE)
	EXTERNAL_GOB_URL      = fmt.Sprintf("https://%s", EXTERNAL_GOB_HOST)

	INTERNAL_GOB_INSTANCE = "chrome-internal"
	INTERNAL_GOB_HOST     = fmt.Sprintf(GOB_HOST, INTERNAL_GOB_INSTANCE)
	INTERNAL_GOB_URL      = fmt.Sprintf("https://%s", INTERNAL_GOB_HOST)

	AOSP_GOB_INSTANCE = "android"
	AOSP_GOB_HOST     = fmt.Sprintf(GOB_HOST, AOSP_GOB_INSTANCE)
	AOSP_GOB_URL      = fmt.Sprintf("https://%s", AOSP_GOB_HOST)

	WEAVE_GOB_INSTANCE = "weave"
	WEAVE_GOB_HOST     = fmt.Sprintf(GOB_HOST, WEAVE_GOB_INSTANCE)
	WEAVE_GOB_URL      = fmt.Sprintf("https://%s", WEAVE_GOB_HOST)

	external_remote = "cros"
	internal_remote = "cros-internal"

	CROS_REMOTES = map[string]string{
		external_remote: EXTERNAL_GOB_URL,
		internal_remote: INTERNAL_GOB_URL,
		"aosp":          AOSP_GOB_URL,
		"weave":         WEAVE_GOB_URL,
	}

	// Mapping 'remote name' -> regexp that matches names of repositories on
	// that remote that can be branched when creating CrOS branch.
	// Branching script will actually create a new git ref when branching
	// these projects. It won't attempt to create a git ref for other projects
	// that may be mentioned in a manifest. If a remote is missing from this
	// dictionary, all projects on that remote are considered to not be
	// branchable.
	BRANCHABLE_PROJECTS = map[string]*regexp.Regexp{
		external_remote: regexp.MustCompile("(chromiumos|aosp)/(.+)"),
		internal_remote: regexp.MustCompile("chromeos/(.+)"),
	}

	MANIFEST_ATTR_BRANCHING_CREATE = "create"
)

type ProjectBranch struct {
	project    repo.Project
	branchName string
}

// canBranchProject retuns true if the project can be branched.
//
// The preferred way to specify branchability is by adding a "branch-mode"
// annotation on the project in the manifest. Of course, only one project
// in the manifest actually does this.

// The legacy method is to peek at the project's remote.
func canBranchProject(manifest repo.Manifest, project repo.Project) bool {
	// Anotation is set.
	explicitMode, _ := project.GetAnnotation("branch-mode")
	if explicitMode != "" {
		return explicitMode == MANIFEST_ATTR_BRANCHING_CREATE
	}

	// Peek at remote.
	remote := manifest.GetRemoteByName(project.RemoteName)
	remoteName := remote.Name
	if remote.Alias != "" {
		remoteName = remote.Alias
	}
	_, inCrosRemote := CROS_REMOTES[remoteName]
	projectRegexp, inBranchableProjects := BRANCHABLE_PROJECTS[remoteName]
	return inCrosRemote && inBranchableProjects && projectRegexp.MatchString(project.Name)
}

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
