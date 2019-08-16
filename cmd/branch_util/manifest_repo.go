// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	"go.chromium.org/luci/common/errors"
)

type ManifestRepo struct {
	ProjectCheckout string
	Project         repo.Project
}

const (
	manifestAttrBranchingTot = "tot"
	defaultManifest          = "default.xml"
	officialManifest         = "official.xml"
)

var loadManifestFromFile = repo.LoadManifestFromFile
var loadManifestTree = repo.LoadManifestTree

func (m *ManifestRepo) gitRevision(project repo.Project) (string, error) {
	if git.IsSHA(project.Revision) {
		return project.Revision, nil
	}

	remoteUrl, err := projectFetchUrl(project.Path)
	if err != nil {
		return "", err
	}

	// Doesn't need to be in an actual git repo.
	output, err := git.RunGit("", []string{"ls-remote", remoteUrl, project.Revision})
	if err != nil {
		return "", errors.Annotate(err, "failed to read remote branches for %s", remoteUrl).Err()
	}
	if strings.TrimSpace(output.Stdout) == "" {
		return "", fmt.Errorf("no ref for %s in project %s", project.Revision, project.Path)
	}
	return strings.Fields(output.Stdout)[0], nil
}

// RepairManifest reads the manifest at the given path and repairs it in memory.
// Because humans rarely read branched manifests, this function optimizes for
// code readibility and explicitly sets revision on every project in the manifest,
// deleting any defaults.
// branchesByPath maps project paths to branch names.
func (m *ManifestRepo) RepairManifest(path string, branchesByPath map[string]string) (*repo.Manifest, error) {
	manifest, err := loadManifestFromFile(path)
	if err != nil {
		return nil, errors.Annotate(err, "error loading manifest").Err()
	}

	// Delete the default revision.
	manifest.Default.Revision = ""

	// Delete remote revisions.
	for i := range manifest.Remotes {
		manifest.Remotes[i].Revision = ""
	}

	// Update all project revisions.
	for i, project := range manifest.Projects {
		workingProject, err := workingManifest.GetProjectByPath(project.Path)
		if err != nil {
			return nil, fmt.Errorf("project %s does not exist in working manifest", project.Path)
		}

		switch branchMode := workingManifest.ProjectBranchMode(project); branchMode {
		case repo.Create:
			branchName, inDict := branchesByPath[project.Path]
			if !inDict {
				return nil, fmt.Errorf("project %s is not pinned/tot but not set in branchesByPath", project.Path)
			}
			manifest.Projects[i].Revision = git.NormalizeRef(branchName)
		case repo.Tot:
			manifest.Projects[i].Revision = git.NormalizeRef("master")
		case repo.Pinned:
			// TODO(@jackneus): all this does is convert the current revision to a SHA.
			// Is this really necessary?
			revision, err := m.gitRevision(*workingProject)
			if err != nil {
				return nil, errors.Annotate(err, "error repairing manifest").Err()
			}
			manifest.Projects[i].Revision = revision
		default:
			return nil, fmt.Errorf("project %s branch mode unspecifed", project.Path)
		}

		manifest.Projects[i].Upstream = ""
	}
	return &manifest, nil
}

// listManifests finds all manifests included directly or indirectly by root
// manifests.
func (m *ManifestRepo) listManifests(rootPaths []string) ([]string, error) {
	manifestPaths := make(map[string]bool)

	for _, path := range rootPaths {
		path = filepath.Join(m.ProjectCheckout, path)
		manifestMap, err := loadManifestTree(path)
		if err != nil {
			// It is only correct to continue when a file does not exist,
			// not because of other errors (like invalid XML).
			if strings.Contains(err.Error(), "failed to open") {
				continue
			} else {
				return []string{}, err
			}
		}
		for k := range manifestMap {
			manifestPaths[filepath.Join(filepath.Dir(path), k)] = true
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
	log.Printf("Repairing manifest project %s.", m.Project.Name)
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
