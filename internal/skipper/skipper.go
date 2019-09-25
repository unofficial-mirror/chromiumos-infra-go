// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package skipper

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	cros_pb "go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

// CheckBuilders determines which builders can be skipped and which must be run.
func CheckBuilders(
	builders []*cros_pb.BuilderConfig,
	changes []*bbproto.GerritChange,
	changeRevs *gerrit.ChangeRevData,
	repoToBranchToSrcRoot map[string]map[string]string,
	cfg testplans_pb.BuildIrrelevanceCfg) (*cros_pb.BuildSkipperResponse, error) {

	response := &cros_pb.BuildSkipperResponse{}

	// Get all of the files referenced by each GerritCommit in the Build.
	affectedFiles, err := extractAffectedFiles(changes, changeRevs, repoToBranchToSrcRoot)
	if err != nil {
		return nil, fmt.Errorf("error in extractAffectedFiles: %+v", err)
	}
	hasAffectedFiles := len(affectedFiles) > 0
	ignoreImageBuilders := ignoreImageBuilders(affectedFiles, cfg)

builderLoop:
	for _, b := range builders {
		if isImageBuilder(b) && ignoreImageBuilders {
			log.Printf("Ignoring %v because it's an image builder and the changes don't affect Portage", b.GetId().GetName())
			response.SkipForGlobalBuildIrrelevance = append(response.SkipForGlobalBuildIrrelevance, b.GetId())
			continue builderLoop
		}
		switch b.GetBuild().GetRunWhen().GetMode() {
		case cros_pb.BuilderConfig_Build_RunWhen_ONLY_RUN_ON_FILE_MATCH:
			if hasAffectedFiles && ignoreByOnlyRunOnFileMatch(affectedFiles, b) {
				log.Printf("For %v, there's a file required by OnlyRunOnFileMatch rules", b.GetId().GetName())
				response.SkipForRunWhenRules = append(response.SkipForRunWhenRules, b.GetId())
				continue builderLoop
			}
		case cros_pb.BuilderConfig_Build_RunWhen_ALWAYS_RUN:
			log.Printf("Builder %v has ALWAYS_RUN RunWhen mode", b.GetId().GetName())
		case cros_pb.BuilderConfig_Build_RunWhen_MODE_UNSPECIFIED:
			log.Printf("Builder %v has MODE_UNSPECIFIED RunWhen mode", b.GetId().GetName())
		}
		log.Printf("Must run builder %v", b.GetId().GetName())
		response.BuildsToRun = append(response.BuildsToRun, b.GetId())
	}
	return response, nil
}

func isImageBuilder(b *cros_pb.BuilderConfig) bool {
	for _, art := range b.GetArtifacts().GetArtifactTypes() {
		if art == cros_pb.BuilderConfig_Artifacts_IMAGE_ZIP {
			return true
		}
	}
	return false
}

func ignoreImageBuilders(affectedFiles []string, cfg testplans_pb.BuildIrrelevanceCfg) bool {
	if len(affectedFiles) == 0 {
		log.Print("Cannot ignore image builders, since no affected files were provided")
		return false
	}
	// Filter out files that are irrelevant to Portage because of the config.
	affectedFiles = filterByBuildIrrelevantPaths(affectedFiles, cfg)
	if len(affectedFiles) == 0 {
		log.Printf("All files ruled out by build-irrelevant paths for builder. " +
			"This means that none of the Gerrit changes in the build input could affect " +
			"the outcome of image builders")
		return true
	}
	log.Printf("After considering build-irrelevant paths, we still must consider "+
		"the following files for image builders:\n%v",
		strings.Join(affectedFiles, "\n"))
	return false
}

func ignoreByOnlyRunOnFileMatch(affectedFiles []string, b *cros_pb.BuilderConfig) bool {
	rw := b.GetBuild().GetRunWhen()
	if rw.GetMode() != cros_pb.BuilderConfig_Build_RunWhen_ONLY_RUN_ON_FILE_MATCH {
		log.Printf("Can't apply OnlyRunOnFileMatch rule to %v, since it has mode %v", b.GetId().GetName(), rw.GetMode())
		return false
	}
	if len(rw.GetFilePatterns()) == 0 {
		log.Printf("Can't apply OnlyRunOnFileMatch rule to %v, since it has empty FilePatterns", b.GetId().GetName())
		return false
	}
	affectedFiles = findFilesMatchingPatterns(affectedFiles, b.GetBuild().GetRunWhen().GetFilePatterns())
	if len(affectedFiles) == 0 {
		return true
	}
	log.Printf("After considering OnlyRunOnFileMatch rules, the following files require builder %v:\n%v",
		b.GetId().GetName(), strings.Join(affectedFiles, "\n"))
	return false
}

func extractAffectedFiles(changes []*bbproto.GerritChange, changeRevs *gerrit.ChangeRevData, repoToSrcRoot map[string]map[string]string) ([]string, error) {
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

func findFilesMatchingPatterns(files []string, patterns []string) []string {
	matchedFiles := make([]string, 0)
affectedFile:
	for _, f := range files {
		for _, pattern := range patterns {
			match, err := doublestar.Match(pattern, f)
			if err != nil {
				log.Fatalf("Failed to match pattern %s against file %s: %v", pattern, f, err)
			}
			if match {
				log.Printf("File %s matches pattern %s", f, pattern)
				matchedFiles = append(matchedFiles, f)
				continue affectedFile
			}
		}
		log.Printf("File %s matches none of the patterns %v", f, patterns)
	}
	return matchedFiles
}
