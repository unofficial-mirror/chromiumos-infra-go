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
	Default  []Default `xml:"default"`
}

// Project is an element of a manifest containing a Gerrit project to source path definition.
type Project struct {
	Path     string `xml:"path,attr"`
	Name     string `xml:"name,attr"`
	Revision string `xml:"revision,attr"`
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
}

// Default is a manifest element that lists the default.
type Default struct {
	Remote   string `xml:"remote,attr"`
	Revision string `xml:"revision,attr"`
}

// LoadManifestFromFile loads the manifest at the given file path into
// a Manifest struct. It also loads all included manifests.
// Returns a map mapping manifest filenames to file contents.
func LoadManifestFromFile(file string) (map[string]*Manifest, error) {
	results := make(map[string]*Manifest)

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open and read %s", file).Err()
	}
	manifest := &Manifest{}
	if err = xml.Unmarshal(data, manifest); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal %s", file).Err()
	}
	results[file] = manifest

	// Recursively fetch manifests listed in "include" elements.
	for _, incl := range manifest.Includes {
		// Include paths are relative to the manifest location.
		inclPath := filepath.Join(filepath.Dir(file), incl.Name)
		subResults, err := LoadManifestFromFile(inclPath)
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
func GetRepoToSourceRootFromManifests(authedClient *http.Client, ctx context.Context, manifestCommit string) (map[string]string, error) {
	manifests, err := fetchManifestRecursive(authedClient, ctx, manifestCommit, rootXml)
	if err != nil {
		return nil, err
	}
	repoToSourceRoot := make(map[string]string)
	for _, m := range manifests {
		for _, p := range m.Projects {
			repoToSourceRoot[p.Name] = p.Path
		}
	}
	log.Printf("Found %d repo to source root mappings from manifest files", len(repoToSourceRoot))
	return repoToSourceRoot, nil
}

// Return the unique project with the given name (nil if the project DNE).
// Return an error if multiple projects with the given name exist.
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
