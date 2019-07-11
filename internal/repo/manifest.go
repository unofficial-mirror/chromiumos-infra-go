// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo

import (
	"context"
	"encoding/xml"
	"fmt"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/luci/common/errors"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

var (
	// Name of the root XML file to seek in manifest-internal.
	rootXml = "snapshot.xml"
)

// Manifest is a top-level Repo definition file.
type Manifest struct {
	Includes []Include `xml:"include"`
	Projects []Project `xml:"project"`
	Remotes  []Remote  `xml:"remote"`
	Default  Default   `xml:"default"`
}

// Project is an element of a manifest containing a Gerrit project to source path definition.
type Project struct {
	Path        string       `xml:"path,attr"`
	Name        string       `xml:"name,attr"`
	Revision    string       `xml:"revision,attr"`
	Upstream    string       `xml:"upstream,attr"`
	RemoteName  string       `xml:"remote,attr"`
	Annotations []Annotation `xml:"annotation"`
}

// Annotation is an element of a manifest annotating the parent element.
type Annotation struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Include is a manifest element that imports another manifest file.
type Include struct {
	Name string `xml:"name,attr"`
}

// Remote is a manifest element that lists a remote.
type Remote struct {
	Fetch    string `xml:"fetch,attr"`
	Name     string `xml:"name,attr"`
	Revision string `xml:"revision,attr"`
	Alias    string `xml:"alias,attr"`
}

// Default is a manifest element that lists the default.
type Default struct {
	RemoteName string `xml:"remote,attr"`
	Revision   string `xml:"revision,attr"`
}

// GitName returns the git name of the remote, which
// is Alias if it is set, and Name otherwise.
func (r *Remote) GitName() string {
	if r.Alias != "" {
		return r.Alias
	}
	return r.Name
}

// GetRemoteByName returns a pointer to the remote with
// the given name/alias in the given manifest.
func (m *Manifest) GetRemoteByName(name string) *Remote {
	for _, remote := range m.Remotes {
		if remote.Name == name {
			return &remote
		}
	}
	return &Remote{}
}

// GetAnnotation returns the value of the annotation with the
// given name, if it exists. It also returns a bool indicating
// whether or not the annotation exists.
func (p *Project) GetAnnotation(name string) (string, bool) {
	for _, annotation := range p.Annotations {
		if annotation.Name == name {
			return annotation.Value, true
		}
	}
	return "", false
}

// LoadManifestFromFile ile loads the manifest at the given file into a
// Manfiest struct.
func LoadManifestFromFile(file string) (Manifest, error) {
	manifestMap, err := LoadManifestTree(file)
	if err != nil {
		return Manifest{}, err
	}
	manifest, exists := manifestMap[file]
	if !exists {
		return Manifest{}, fmt.Errorf("failed to read %s", file)
	}
	return *manifest, nil
}

// LoadManifestTree loads the manifest at the given file path into
// a Manifest struct. It also loads all included manifests.
// Returns a map mapping manifest filenames to file contents.
func LoadManifestTree(file string) (map[string]*Manifest, error) {
	results := make(map[string]*Manifest)

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open and read %s", file).Err()
	}
	manifest := &Manifest{}
	if err = xml.Unmarshal(data, manifest); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal %s", file).Err()
	}
	for i, project := range manifest.Projects {
		// Set default remote on projects without an explicit remote
		if project.RemoteName == "" {
			project.RemoteName = manifest.Default.RemoteName
		}
		// Set default revision on projects without an explicit revision
		if project.Revision == "" {
			remote := manifest.GetRemoteByName(project.RemoteName)
			if remote.Revision == "" {
				project.Revision = manifest.Default.Revision
			} else {
				project.Revision = remote.Revision
			}
		}
		manifest.Projects[i] = project
	}
	results[file] = manifest

	// Recursively fetch manifests listed in "include" elements.
	for _, incl := range manifest.Includes {
		// Include paths are relative to the manifest location.
		inclPath := filepath.Join(filepath.Dir(file), incl.Name)
		subResults, err := LoadManifestTree(inclPath)
		if err != nil {
			return nil, err
		}
		for k, v := range subResults {
			results[k] = v
		}
	}
	return results, nil
}

func fetchManifestRecursive(authedClient *http.Client, ctx context.Context, manifestCommit string, file string) (map[string]*Manifest, error) {
	results := make(map[string]*Manifest)
	log.Printf("Fetching manifest file %s at revision '%s'", file, manifestCommit)
	files, err := gerrit.FetchFilesFromGitiles(
		authedClient,
		ctx,
		"chrome-internal.googlesource.com",
		"chromeos/manifest-internal",
		manifestCommit,
		[]string{file})
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch %s", file).Err()
	}
	manifest := &Manifest{}
	if err = xml.Unmarshal([]byte((*files)[file]), manifest); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal %s", file).Err()
	}
	results[file] = manifest
	// Recursively fetch manifests listed in "include" elements.
	for _, incl := range manifest.Includes {
		subResults, err := fetchManifestRecursive(authedClient, ctx, manifestCommit, incl.Name)
		if err != nil {
			return nil, err
		}
		for k, v := range subResults {
			results[k] = v
		}
	}
	return results, nil
}

// GetRepoToSourceRootFromManifests constructs a Gerrit project to path mapping by fetching manifest
// XML files from Gitiles.
func GetRepoToRemoteBranchToSourceRootFromManifests(authedClient *http.Client, ctx context.Context, manifestCommit string) (map[string]map[string]string, error) {
	manifests, err := fetchManifestRecursive(authedClient, ctx, manifestCommit, rootXml)
	if err != nil {
		return nil, err
	}
	repoToSourceRoot := make(map[string]map[string]string)
	for _, m := range manifests {
		for _, p := range m.Projects {
			if _, found := repoToSourceRoot[p.Name]; !found {
				repoToSourceRoot[p.Name] = make(map[string]string)
			}
			branch := p.Upstream
			if branch == "" {
				branch = "refs/heads/master"
			}
			if !strings.HasPrefix(branch, "refs/heads/") {
				branch = "refs/heads/" + branch
			}
			repoToSourceRoot[p.Name][branch] = p.Path
		}
	}
	log.Printf("Found %d repo to source root mappings from manifest files", len(repoToSourceRoot))
	return repoToSourceRoot, nil
}

// GetUnique Project returns the unique project with the given name
// (nil if the project DNE). It returns an error if multiple projects with the
// given name exist.
func (m *Manifest) GetUniqueProject(name string) (Project, error) {
	var project Project
	matchingProjects := 0
	for _, p := range m.Projects {
		if p.Name == name {
			matchingProjects++
			if matchingProjects > 1 {
				return Project{}, fmt.Errorf("multiple projects named %s", name)
			}
			project = p
		}
	}
	return project, nil
}

// Write writes the manifest to the given path.
func (m *Manifest) Write(path string) error {
	data, err := xml.Marshal(m)
	if err != nil {
		return errors.Annotate(err, "failed to write manifest").Err()
	}
	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return errors.Annotate(err, "failed to write manifest").Err()
	}
	return nil
}
