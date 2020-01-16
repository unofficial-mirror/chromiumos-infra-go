// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package pointless

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar"
	"github.com/golang/protobuf/ptypes/wrappers"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

// CheckBuilder assesses whether a child builder is pointless for a given CQ run. This may be the
// case if the commits in the CQ run don't affect any files that could possibly affect this
// builder's relevant paths.
func CheckBuilder(
	changes []*bbproto.GerritChange,
	changeRevs *gerrit.ChangeRevData,
	relevantPaths []*testplans_pb.PointlessBuildCheckRequest_Path,
	repoToBranchToSrcRoot map[string]map[string]string,
	cfg testplans_pb.BuildIrrelevanceCfg) (*testplans_pb.PointlessBuildCheckResponse, error) {

	// Get all of the files referenced by each GerritCommit in the Build.
	affectedFiles, err := extractAffectedFiles(changes, changeRevs, repoToBranchToSrcRoot)
	if err != nil {
		return nil, fmt.Errorf("error in extractAffectedFiles: %+v", err)
	}
	if len(affectedFiles) == 0 {
		log.Printf("No affected files, so this can't be a CQ run. " +
			"Aborting with BuildIsPointless := false")
		return &testplans_pb.PointlessBuildCheckResponse{
			BuildIsPointless: &wrappers.BoolValue{Value: false},
		}, nil
	}

	// Filter out files that are irrelevant to Portage because of the config.
	affectedFiles = filterByBuildIrrelevantPaths(affectedFiles, cfg)
	if len(affectedFiles) == 0 {
		log.Printf("All files ruled out by build-irrelevant paths. This means that " +
			"none of the Gerrit changes in the build input could affect the outcome of the build")
		return &testplans_pb.PointlessBuildCheckResponse{
			BuildIsPointless:     &wrappers.BoolValue{Value: true},
			PointlessBuildReason: testplans_pb.PointlessBuildCheckResponse_IRRELEVANT_TO_KNOWN_NON_PORTAGE_DIRECTORIES,
		}, nil
	}
	log.Printf("After considering build-irrelevant paths, we still must consider files:\n%v",
		strings.Join(affectedFiles, "\n"))

	// Filter out files that aren't in the relevant paths.
	affectedFiles = filterByPortageDeps(affectedFiles, relevantPaths)
	if len(affectedFiles) == 0 {
		log.Printf("All files ruled out after checking relevant paths")
		return &testplans_pb.PointlessBuildCheckResponse{
			BuildIsPointless:     &wrappers.BoolValue{Value: true},
			PointlessBuildReason: testplans_pb.PointlessBuildCheckResponse_IRRELEVANT_TO_DEPS_GRAPH,
		}, nil
	}

	log.Printf("This build is not pointless, due to files:\n%v",
		strings.Join(affectedFiles, "\n"))
	return &testplans_pb.PointlessBuildCheckResponse{
		BuildIsPointless: &wrappers.BoolValue{Value: false},
	}, nil
}

func extractAffectedFiles(changes []*bbproto.GerritChange,
	changeRevs *gerrit.ChangeRevData, repoToSrcRoot map[string]map[string]string) ([]string, error) {
	allAffectedFiles := make([]string, 0)
	for _, gc := range changes {
		rev, err := changeRevs.GetChangeRev(gc.Host, gc.Change, int32(gc.Patchset))
		if err != nil {
			return nil, err
		}
		branchMapping, found := repoToSrcRoot[rev.Project]
		if !found {
			return nil, fmt.Errorf("Found no branch mapping for project %s", rev.Project)
		}
		srcRootMapping, found := branchMapping[rev.Branch]
		if !found {
			return nil, fmt.Errorf("Found no source mapping for project %s and branch %s", rev.Project, rev.Branch)
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

func filterByBuildIrrelevantPaths(files []string, cfg testplans_pb.BuildIrrelevanceCfg) []string {
	pipFilteredFiles := make([]string, 0)
affectedFile:
	for _, f := range files {
		for _, pattern := range cfg.IrrelevantFilePatterns {
			match, err := doublestar.Match(pattern.Pattern, f)
			if err != nil {
				log.Fatalf("Failed to match pattern %s against file %s: %v", pattern, f, err)
			}
			if match {
				log.Printf("Ignoring file %s, since it matches Portage irrelevant pattern %s", f, pattern.Pattern)
				continue affectedFile
			}
		}
		log.Printf("Cannot ignore file %s by Portage irrelevant path rules", f)
		pipFilteredFiles = append(pipFilteredFiles, f)
	}
	return pipFilteredFiles
}

func filterByPortageDeps(files []string, relevantPaths []*testplans_pb.PointlessBuildCheckRequest_Path) []string {
	portageDeps := make([]string, 0)
	for _, path := range relevantPaths {
		portageDeps = append(portageDeps, path.Path)
	}
	log.Printf("Found %d affected files to consider:\n"+
		"<portage dep paths>\n%v\n</portage dep paths>",
		len(portageDeps), strings.Join(portageDeps, "\n"))

	portageFilteredFiles := make([]string, 0)
affectedFile:
	for _, f := range files {
		for _, pd := range portageDeps {
			if f == pd {
				log.Printf("Cannot ignore file %s due to Portage dependency %s", f, pd)
				portageFilteredFiles = append(portageFilteredFiles, f)
				continue affectedFile
			}
			pdAsDir := strings.TrimSuffix(pd, "/") + "/"
			if strings.HasPrefix(f, pdAsDir) {
				log.Printf("Cannot ignore file %s since it's in Portage dependency %s", f, pd)
				portageFilteredFiles = append(portageFilteredFiles, f)
				continue affectedFile
			}
		}
		log.Printf("Ignoring file %s because no prefix of it is referenced in the relevant paths", f)
	}
	return portageFilteredFiles
}
