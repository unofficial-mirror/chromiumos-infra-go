// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package manifest_repo

import (
	checkoutp "go.chromium.org/chromiumos/infra/go/internal/checkout"
	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

type ManifestRepo struct {
	checkout checkoutp.Checkout
	project  repo.Project
}

const (
	manifestAttrBranchingTot = "tot"
	defaultManifest          = "default.xml"
	officialManifest         = "official.xml"
)

var loadManifestFromFile = repo.LoadManifestFromFile
var loadManifestTree = repo.LoadManifestTree

// RepairManifest reads the manifest at the given path and repairs it in memory.
// Because humans rarely read branched manifests, this function optimizes for
// code readibility and explicitly sets revision on every project in the manifest,
// deleting any defaults.
// branchesByPath maps project paths to branch names.
func (m *ManifestRepo) RepairManifest(path string, branchesByPath map[string]string) (repo.Manifest, error) {
	manifest, err := loadManifestFromFile(path)
	if err != nil {
		return repo.Manifest{}, errors.Annotate(err, "error repairing manifest").Err()
	}

	// Delete the default revision.
	manifest.Default.Revision = ""

	// Delete remote revisions.
	for i := range manifest.Remotes {
		manifest.Remotes[i].Revision = ""
	}

	// Update all project revisions.
	for i, project := range manifest.Projects {
		err = m.checkout.EnsureProject(project)
		if err != nil {
			return repo.Manifest{}, errors.Annotate(err, "missing project while repairing manifest").Err()
		}

		// If project path is in the dict, the project must have been branched.
		branchName, inDict := branchesByPath[project.Path]
		explicitMode, _ := project.GetAnnotation("branch-mode")

		if inDict {
			manifest.Projects[i].Revision = git.NormalizeRef(branchName)
		} else if explicitMode == manifestAttrBranchingTot {
			// Otherwise, check if project is explicitly TOT.
			manifest.Projects[i].Revision = git.NormalizeRef("master")
		} else {
			// If not, it's pinned.
			revision, err := m.checkout.GitRevision(project)
			if err != nil {
				return repo.Manifest{}, errors.Annotate(err, "error repairing manifest").Err()
			}
			manifest.Projects[i].Revision = revision
		}

		manifest.Projects[i].Upstream = ""
	}
	return manifest, nil
}

// listManifests finds all manifests included directly or indirectly by root
// manifests.
func (m *ManifestRepo) listManifests(rootPaths []string) ([]string, error) {
	manifestPaths := make(map[string]bool)

	for _, path := range rootPaths {
		path = m.checkout.AbsoluteProjectPath(m.project, path)
		manifestMap, err := loadManifestTree(path)
		if err != nil {
			return []string{}, err
		}
		for k := range manifestMap {
			manifestPaths[k] = true
		}
	}
	manifests := []string{}
	for k := range manifestPaths {
		manifests = append(manifests, k)
	}
	return manifests, nil
}

// RepairManifestsOnDisk repairs the revision and upstream attributes of
// manifest elements on disk for the given projects.
func (m *ManifestRepo) RepairManifestsOnDisk(branchesByPath map[string]string) error {
	manifestPaths, err := m.listManifests([]string{defaultManifest, officialManifest})
	if err != nil {
		return errors.Annotate(err, "failed to listManifests").Err()
	}
	for _, manifestPath := range manifestPaths {
		manifest, err := m.RepairManifest(manifestPath, branchesByPath)
		if err != nil {
			return errors.Annotate(err, "failed to repair manifest %s", manifestPath).Err()
		}
		err = manifest.Write(manifestPath)
		if err != nil {
			return errors.Annotate(err, "failed to write repaired manifest to %s", manifestPath).Err()
		}
	}
	return nil
}
