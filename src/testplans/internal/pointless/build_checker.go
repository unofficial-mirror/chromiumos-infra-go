// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package pointless

import (
	"fmt"
	"github.com/golang/protobuf/ptypes/wrappers"
	chromite "go.chromium.org/chromiumos/infra/proto/go/chromite/api"
	"go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"log"
	"sort"
	"strings"
	"testplans/internal/git"
)

const (
	// The location of the repo checkout inside the chroot.
	// This prefix needs to be trimmed from dependencySourcePaths at the moment, but that will become
	// unnecessary once https://crbug.com/957090 is resolved.
	depSourcePathPrefix = "/mnt/host/source/"
)

// CheckBuilder assesses whether a child builder is pointless for a given CQ run. This may be the
// case if the commits in the CQ run don't affect any files that could possibly affect this
// builder's Portage graph.
func CheckBuilder(
	build *bbproto.Build,
	changeRevs *git.ChangeRevData,
	depGraph *chromite.DepGraph,
	repoToSrcRoot map[string]string,
	buildIrrelevantPaths []string) (*chromiumos.PointlessBuildCheckResponse, error) {

	// Get all of the files referenced by each GerritCommit in the Build.
	affectedFiles, err := extractAffectedFiles(build, changeRevs, repoToSrcRoot)
	if err != nil {
		return nil, err
	}
	if len(affectedFiles) == 0 {
		log.Printf("Build %s: No affected files, so this can't be a CQ run. "+
			"Aborting with BuildIsPointless := false", getBuildTarget(build))
		return &chromiumos.PointlessBuildCheckResponse{
			BuildIsPointless: &wrappers.BoolValue{Value: false},
		}, nil
	}

	// Filter out files that are irrelevant to Portage because of the BuildIrrelevantPaths.
	affectedFiles = filterByBuildIrrelevantPaths(affectedFiles, buildIrrelevantPaths)
	if len(affectedFiles) == 0 {
		log.Printf("Build %s: All files ruled out by build-irrelevant paths. This means that "+
			"none of the Gerrit changes in the build input could affect the outcome of the build",
			getBuildTarget(build))
		return &chromiumos.PointlessBuildCheckResponse{
			BuildIsPointless:     &wrappers.BoolValue{Value: true},
			PointlessBuildReason: chromiumos.PointlessBuildCheckResponse_IRRELEVANT_TO_KNOWN_NON_PORTAGE_DIRECTORIES,
		}, nil
	}
	log.Printf("After considering build-irrelevant paths, we still must consider files:\n%v",
		strings.Join(affectedFiles, "\n"))

	// Filter out files that aren't in the Portage dep graph.
	affectedFiles = filterByPortageDeps(affectedFiles, depGraph)
	if len(affectedFiles) == 0 {
		log.Printf("Build %s: All files ruled out after checking dep graph", getBuildTarget(build))
		return &chromiumos.PointlessBuildCheckResponse{
			BuildIsPointless:     &wrappers.BoolValue{Value: true},
			PointlessBuildReason: chromiumos.PointlessBuildCheckResponse_IRRELEVANT_TO_DEPS_GRAPH,
		}, nil
	}

	log.Printf("Build %s: This build is not pointless, due to files:\n%v",
		getBuildTarget(build), strings.Join(affectedFiles, "\n"))
	return &chromiumos.PointlessBuildCheckResponse{
		BuildIsPointless: &wrappers.BoolValue{Value: false},
	}, nil
}

func extractAffectedFiles(build *bbproto.Build,
	changeRevs *git.ChangeRevData, repoToSrcRoot map[string]string) ([]string, error) {
	allAffectedFiles := make([]string, 0)
	for _, gc := range build.Input.GerritChanges {
		rev, err := changeRevs.GetChangeRev(gc.Host, gc.Change, int32(gc.Patchset))
		if err != nil {
			return nil, err
		}
		srcRootMapping, found := repoToSrcRoot[rev.Project]
		if !found {
			return nil, fmt.Errorf("Found no source mapping for project %s", rev.Project)
		}
		affectedFiles := make([]string, 0, len(rev.Files))
		for _, file := range rev.Files {
			fileSrcPath := fmt.Sprintf("%s/%s", srcRootMapping, file)
			affectedFiles = append(affectedFiles, fileSrcPath)
		}
		sort.Strings(affectedFiles)
		log.Printf("For https://%s/%d, affected files:\n%v\n\n",
			gc.Host, gc.Change, strings.Join(affectedFiles, "\n"))
		allAffectedFiles = append(allAffectedFiles, affectedFiles...)
	}
	sort.Strings(allAffectedFiles)
	log.Printf("All affected files:\n%v\n\n", strings.Join(allAffectedFiles, "\n"))
	return allAffectedFiles, nil
}

func filterByBuildIrrelevantPaths(files, portageIrrelevantPaths []string) []string {
	pipFilteredFiles := make([]string, 0)
affectedFile:
	for _, f := range files {
		for _, pip := range portageIrrelevantPaths {
			if strings.HasPrefix(f, pip) {
				log.Printf("Ignoring file %s, since it's contained in Portage irrelevant path %s", f, pip)
				continue affectedFile
			}
		}
		log.Printf("Cannot ignore file %s by Portage irrelevant path rules", f)
		pipFilteredFiles = append(pipFilteredFiles, f)
	}
	return pipFilteredFiles
}

func filterByPortageDeps(files []string, depGraph *chromite.DepGraph) []string {
	portageDeps := make([]string, 0)
	for _, pd := range depGraph.PackageDeps {
		for _, sp := range pd.DependencySourcePaths {
			portageDeps = append(portageDeps, strings.TrimPrefix(sp.Path, depSourcePathPrefix))
		}
	}
	log.Printf("Found %d Portage deps to consider from the build graph:\n"+
		"<portage dep paths>\n%v\n</portage dep paths>",
		len(portageDeps), strings.Join(portageDeps, "\n"))

	portageFilteredFiles := make([]string, 0)
affectedFile:
	for _, f := range files {
		for _, pd := range portageDeps {
			if strings.HasPrefix(f, pd) {
				log.Printf("Cannot ignore file %s due to Portage dependency %s", f, pd)
				portageFilteredFiles = append(portageFilteredFiles, f)
				continue affectedFile
			}
		}
		log.Printf("Ignoring file %s because no prefix of it is referenced in the dep graph", f)
	}
	return portageFilteredFiles
}

func getBuildTarget(bb *bbproto.Build) string {
	return bb.Output.Properties.Fields["build_target"].GetStructValue().Fields["name"].GetStringValue()
}
