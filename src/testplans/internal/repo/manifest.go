package repo

import (
	"context"
	"encoding/xml"
	"go.chromium.org/luci/common/errors"
	"log"
	"net/http"
	"testplans/internal/git"
)

var (
	// Name of the root XML file to seek in manifest-internal.
	rootXml = "snapshot.xml"
)

// Manifest is a top-level Repo definition file.
type Manifest struct {
	Includes []Include `xml:"include"`
	Projects []Project `xml:"project"`
}

// Project is an element of a manifest containing a Gerrit project to source path definition.
type Project struct {
	Path string `xml:"path,attr"`
	Name string `xml:"name,attr"`
}

// Include is a manifest element that imports another manifest file.
type Include struct {
	Name string `xml:"name,attr"`
}

func fetchManifestRecursive(authedClient *http.Client, ctx context.Context, manifestCommit string, file string) (map[string]*Manifest, error) {
	results := make(map[string]*Manifest)
	log.Printf("Fetching manifest file %s at revision '%s'", file, manifestCommit)
	files, err := git.FetchFilesFromGitiles(
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
