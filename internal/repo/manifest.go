// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

var (
	// Name of the root XML file to seek in manifest-internal.
	rootXml = "snapshot.xml"
)

// Manifest is a top-level Repo definition file.
type Manifest struct {
	XMLName   xml.Name    `xml:"manifest"`
	Includes  []Include   `xml:"include"`
	Remotes   []Remote    `xml:"remote"`
	Default   Default     `xml:"default"`
	Notice    string      `xml:"notice,omitempty"`
	RepoHooks []RepoHooks `xml:"repo-hooks"`
	Projects  []Project   `xml:"project"`
}

// Project is an element of a manifest containing a Gerrit project to source path definition.
type Project struct {
	Path        string       `xml:"path,attr,omitempty"`
	Name        string       `xml:"name,attr,omitempty"`
	Revision    string       `xml:"revision,attr,omitempty"`
	Upstream    string       `xml:"upstream,attr,omitempty"`
	RemoteName  string       `xml:"remote,attr,omitempty"`
	Annotations []Annotation `xml:"annotation"`
	Groups      string       `xml:"groups,attr,omitempty"`
	SyncC       string       `xml:"sync-c,attr,omitempty"`
	CopyFiles   []CopyFile   `xml:"copyfile"`
}

// Annotation is an element of a manifest annotating the parent element.
type Annotation struct {
	Name  string `xml:"name,attr,omitempty"`
	Value string `xml:"value,attr,omitempty"`
}

// Include is a manifest element that imports another manifest file.
type Include struct {
	Name string `xml:"name,attr,omitempty"`
}

// Remote is a manifest element that lists a remote.
type Remote struct {
	Fetch    string `xml:"fetch,attr,omitempty"`
	Name     string `xml:"name,attr,omitempty"`
	Revision string `xml:"revision,attr,omitempty"`
	Alias    string `xml:"alias,attr,omitempty"`
}

// Default is a manifest element that lists the default.
type Default struct {
	RemoteName string `xml:"remote,attr,omitempty"`
	Revision   string `xml:"revision,attr,omitempty"`
	SyncJ      string `xml:"sync-j,attr,omitempty"`
}

type CopyFile struct {
	Dest string `xml:"dest,attr,omitempty"`
	Src  string `xml:"src,attr,omitempty"`
}

type RepoHooks struct {
	EnabledList string `xml:"enabled-list,attr,omitempty"`
	InProject   string `xml:"in-project,attr,omitempty"`
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
	for i, remote := range m.Remotes {
		if remote.Name == name {
			return &m.Remotes[i]
		}
	}
	return nil
}

// GetProjectByName returns a pointer to the remote with
// the given path in the given manifest.
func (m *Manifest) GetProjectByName(name string) (*Project, error) {
	for i, project := range m.Projects {
		if project.Name == name {
			return &m.Projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %s does not exist in manifest", name)
}

// GetProjectByPath returns a pointer to the remote with
// the given path in the given manifest.
func (m *Manifest) GetProjectByPath(path string) (*Project, error) {
	for i, project := range m.Projects {
		if project.Path == path {
			return &m.Projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %s does not exist in manifest", path)
}

type projectType string

const (
	singleCheckout projectType = "single"
	multiCheckout  projectType = "multi"
	pinned         projectType = "pinned"
	tot            projectType = "tot"
)

func (m *Manifest) getProjects(ptype projectType) []*Project {
	projectCount := make(map[string]int)

	for _, project := range m.Projects {
		projectCount[project.Name] += 1
	}

	projects := []*Project{}
	for i, project := range m.Projects {
		includeProject := false
		projectMode := m.ProjectBranchMode(project)
		if projectMode == Pinned {
			includeProject = ptype == pinned
		} else if projectMode == Tot {
			includeProject = ptype == tot
		} else if projectCount[project.Name] == 1 {
			includeProject = ptype == singleCheckout
		}
		// Restart the if/else if block here because it is possible
		// to have a project with multiple checkouts, some of which
		// are pinned/tot and some of which are not.
		if projectCount[project.Name] > 1 {
			includeProject = includeProject || ptype == multiCheckout
		}
		if includeProject {
			projects = append(projects, &m.Projects[i])
		}
	}
	return projects
}

// GetSingleCheckoutProjects returns all projects in the manifest that have a
// single checkout and are not pinned/tot.
func (m *Manifest) GetSingleCheckoutProjects() []*Project {
	return m.getProjects(singleCheckout)
}

// GetMultiCheckoutProjects returns all projects in the manifest that have a
// multiple checkouts and are not pinned/tot.
func (m *Manifest) GetMultiCheckoutProjects() []*Project {
	return m.getProjects(multiCheckout)
}

// GetPinnedProjects returns all projects in the manifest that are
// pinned.
func (m *Manifest) GetPinnedProjects() []*Project {
	return m.getProjects(pinned)
}

// GetTotProjects returns all projects in the manifest that are
// tot.
func (m *Manifest) GetTotProjects() []*Project {
	return m.getProjects(tot)
}

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
	MANIFEST_ATTR_BRANCHING_PIN    = "pin"
	MANIFEST_ATTR_BRANCHING_TOT    = "tot"
)

type BranchMode string

const (
	UnspecifiedMode BranchMode = "unspecified"
	Pinned          BranchMode = "pinned"
	Tot             BranchMode = "tot"
	Create          BranchMode = "create"
)

// ProjectBranchMode returns the branch mode (create, pinned, tot) of a project.
func (m *Manifest) ProjectBranchMode(project Project) BranchMode {
	// Anotation is set.
	explicitMode, _ := project.GetAnnotation("branch-mode")
	if explicitMode != "" {
		switch explicitMode {
		case MANIFEST_ATTR_BRANCHING_CREATE:
			return Create
		case MANIFEST_ATTR_BRANCHING_PIN:
			return Pinned
		case MANIFEST_ATTR_BRANCHING_TOT:
			return Tot
		default:
			return UnspecifiedMode
		}
	}

	// Othwerise, peek at remote.
	remote := m.GetRemoteByName(project.RemoteName)
	if remote == nil {
		return UnspecifiedMode
	}
	remoteName := remote.GitName()
	_, inCrosRemote := CROS_REMOTES[remoteName]
	projectRegexp, inBranchableProjects := BRANCHABLE_PROJECTS[remoteName]

	if inCrosRemote && inBranchableProjects && projectRegexp.MatchString(project.Name) {
		return Create
	} else {
		return Pinned
	}
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

// LoadManifestFromFile loads the manifest at the given file into a
// Manifest struct.
func LoadManifestFromFile(file string) (Manifest, error) {
	manifestMap, err := LoadManifestTree(file)
	if err != nil {
		return Manifest{}, err
	}
	manifest, exists := manifestMap[filepath.Base(file)]
	if !exists {
		return Manifest{}, fmt.Errorf("failed to read %s", file)
	}
	return *manifest, nil
}

// LoadManifestFromFileRaw loads the manifest at the given file and returns
// the file contents as a byte array.
func LoadManifestFromFileRaw(file string) ([]byte, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open and read %s", file).Err()
	}
	// We don't ResolveImplicitLinks or otherwise do anything else in this function (as it returns "raw" data).
	return data, nil
}

// LoadManifestFromFileWithIncludes loads the manifest at the given files but also
// calls MergeManifests to resolve includes.
func LoadManifestFromFileWithIncludes(file string) (*Manifest, error) {
	manifestMap, err := LoadManifestTree(file)
	if err != nil {
		return nil, err
	}
	manifest, err := MergeManifests(filepath.Base(file), &manifestMap)
	return manifest, err
}

// ResolveImplicitLinks explicitly sets remote/revision information
// for each project in the manifest.
func (m *Manifest) ResolveImplicitLinks() *Manifest {
	newManifest := *m
	for i, project := range m.Projects {
		// Set default remote on projects without an explicit remote
		if project.RemoteName == "" {
			project.RemoteName = m.Default.RemoteName
		}
		// Set default revision on projects without an explicit revision
		if project.Revision == "" {
			remote := m.GetRemoteByName(project.RemoteName)
			if remote == nil || remote.Revision == "" {
				project.Revision = m.Default.Revision
			} else {
				project.Revision = remote.Revision
			}
		}
		// Path defaults to name.
		if project.Path == "" {
			project.Path = project.Name
		}
		newManifest.Projects[i] = project
	}
	return &newManifest
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
	manifest.XMLName = xml.Name{}
	manifest = manifest.ResolveImplicitLinks()
	results[filepath.Base(file)] = manifest

	// Recursively fetch manifests listed in "include" elements.
	for _, incl := range manifest.Includes {
		// Include paths are relative to the manifest location.
		inclPath := filepath.Join(filepath.Dir(file), incl.Name)
		subResults, err := LoadManifestTree(inclPath)
		if err != nil {
			return nil, err
		}
		for k, v := range subResults {
			results[filepath.Join(filepath.Dir(incl.Name), k)] = v
		}
	}
	return results, nil
}

func fetchManifestRecursive(authedClient *http.Client, ctx context.Context, gc *bbproto.GitilesCommit, file string) (map[string]*Manifest, error) {
	results := make(map[string]*Manifest)
	log.Printf("Fetching manifest file %s at revision '%s'", file, gc.Id)
	files, err := gerrit.FetchFilesFromGitiles(
		authedClient,
		ctx,
		gc.Host,
		gc.Project,
		gc.Id,
		[]string{file})
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch %s", file).Err()
	}
	manifest := &Manifest{}
	if err = xml.Unmarshal([]byte((*files)[file]), manifest); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal %s", file).Err()
	}
	manifest.XMLName = xml.Name{}
	results[file] = manifest
	// Recursively fetch manifests listed in "include" elements.
	for _, incl := range manifest.Includes {
		subResults, err := fetchManifestRecursive(authedClient, ctx, gc, incl.Name)
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
func GetRepoToRemoteBranchToSourceRootFromManifests(authedClient *http.Client, ctx context.Context, gc *bbproto.GitilesCommit) (map[string]map[string]string, error) {
	manifests, err := fetchManifestRecursive(authedClient, ctx, gc, rootXml)
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
	if matchingProjects == 0 {
		return Project{}, fmt.Errorf("no project named %s", name)
	}
	return project, nil
}

// Write writes the manifest to the given path.
func (m *Manifest) Write(path string) error {
	data, err := xml.MarshalIndent(m, "", "  ")
	if err != nil {
		return errors.Annotate(err, "failed to write manifest").Err()
	}
	err = ioutil.WriteFile(path, []byte(xml.Header+string(data)), 0644)
	if err != nil {
		return errors.Annotate(err, "failed to write manifest").Err()
	}
	return nil
}

func projectInArr(project Project, projects []Project) bool {
	for _, proj := range projects {
		// Path is a unniue identifier for projects.
		if project.Path == proj.Path {
			return true
		}
	}
	return false
}

func remoteInArr(remote Remote, remotes []Remote) bool {
	for _, rem := range remotes {
		if remote.Name == rem.Name {
			return true
		}
	}
	return false
}

// MergeManifests will merge the given manifests based on includes, taking
// manifests[path] to be the top-level manifest.
// manifests maps manifest filenames to the Manifest structs themselves.
// This basically re-implements `repo manifest` but is necessary because we can't run
// `repo manifest` on a singular git repository.
func MergeManifests(root string, manifests *map[string]*Manifest) (*Manifest, error) {
	baseManifest, ok := (*manifests)[root]
	if !ok {
		return nil, fmt.Errorf("manifest %s does not exist", root)
	}

	// Merge each included manifest. Then merge into baseManifest.
	for _, includedManifest := range baseManifest.Includes {
		mergedManifest, err := MergeManifests(includedManifest.Name, manifests)
		if err != nil {
			return nil, err
		}
		// Merge projects.
		for _, project := range mergedManifest.Projects {
			if !projectInArr(project, baseManifest.Projects) {
				baseManifest.Projects = append(baseManifest.Projects, project)
			}
		}
		// Merge remotes.
		for _, remote := range mergedManifest.Remotes {
			if !remoteInArr(remote, baseManifest.Remotes) {
				baseManifest.Remotes = append(baseManifest.Remotes, remote)
			}
		}
		// Keep the base manifest's default.
	}

	// Clear includes.
	baseManifest.Includes = []Include{}
	baseManifest = baseManifest.ResolveImplicitLinks()
	return baseManifest, nil
}
