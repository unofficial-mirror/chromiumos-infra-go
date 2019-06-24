// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package generator

import (
	"fmt"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"log"
	"strings"
	"testplans/internal/git"
)

type testType int

const (
	hw testType = iota
	vm
)

var (
	testTypeFilter = map[testType]func(testReqs *testplans.TestRestriction) bool{
		hw: func(testReqs *testplans.TestRestriction) bool { return testReqs.DisableHwTests },
		vm: func(testReqs *testplans.TestRestriction) bool { return testReqs.DisableVmTests },
	}
)

func (tt testType) String() string {
	return [...]string{"Hw", "Vm"}[tt]
}

// extractSkippableTests maps BuildTargets to the test types that can be skipped for those targets,
// based on source tree test restrictions.
func extractSkippableTests(
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	buildReports []*bbproto.Build,
	changeRevs *git.ChangeRevData,
	repoToSrcRoot map[string]string) (map[BuildTarget]map[testType]bool, error) {

	skippableTests := make(map[BuildTarget]map[testType]bool)
	for _, report := range buildReports {
		buildTarget := BuildTarget(getBuildTarget(report))
		skippableTests[buildTarget] = make(map[testType]bool)

		disableHWTesting, err := canDisableTesting(sourceTreeCfg, report, changeRevs, repoToSrcRoot, hw)
		if err != nil {
			return skippableTests, err
		}
		skippableTests[buildTarget][hw] = disableHWTesting
		disableVMTesting, err := canDisableTesting(sourceTreeCfg, report, changeRevs, repoToSrcRoot, vm)
		if err != nil {
			return skippableTests, err
		}
		skippableTests[buildTarget][vm] = disableVMTesting
		log.Printf("For build %s, got disableHWTesting = %t, disableVMTesting = %t", getBuildTarget(report), disableHWTesting, disableVMTesting)
	}
	return skippableTests, nil
}


// canDisableTesting determines whether a particular testing type is unnecessary for a given
// builder, based on source tree test restrictions.
func canDisableTesting(
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	buildResult *bbproto.Build,
	changeRevs *git.ChangeRevData,
	repoToSrcRoot map[string]string,
	tt testType) (bool, error) {
	if len(buildResult.Input.GerritChanges) == 0 {
		// This happens during postsubmit runs, for example.
		log.Printf("build doesn't contain gerrit_changes %s, so no tests will be skipped", getBuildTarget(buildResult))
		return false, nil
	}
	fileCount := 0
	for _, commit := range buildResult.Input.GerritChanges {
		chRev, err := changeRevs.GetChangeRev(commit.Host, commit.Change, int32(commit.Patchset))
		if err != nil {
			return false, err
		}
		for _, file := range chRev.Files {
			fileCount++
			srcRootMapping, found := repoToSrcRoot[chRev.Project]
			if !found {
				return false, fmt.Errorf("Found no source mapping for project %s", chRev.Project)
			}
			fileSrcPath := fmt.Sprintf("%s/%s", srcRootMapping, file)
			disableTesting, err := canDisableTestingForPath(fileSrcPath, sourceTreeCfg, tt)
			if err != nil {
				return false, err
			}
			if !disableTesting {
				log.Printf("Can't disable %s testing due to file %s", tt, fileSrcPath)
				return false, nil
			}
		}
	}
	// Either testing is disabled for all of the files or there are zero files.
	log.Printf("%s testing is not needed for %d files in the %s build", tt, fileCount, getBuildTarget(buildResult))
	return true, nil
}

// canDisableTestingForPath determines whether a particular testing type is unnecessary for
// a given file, based on source tree test restrictions.
func canDisableTestingForPath(sourceTreePath string, sourceTreeCfg *testplans.SourceTreeTestCfg, tt testType) (bool, error) {
	for _, sourceTreeRestriction := range sourceTreeCfg.SourceTreeTestRestriction {
		testFilter, ok := testTypeFilter[tt]
		if !ok {
			return false, fmt.Errorf("Missing test filter for %v", tt)
		}
		if testFilter(sourceTreeRestriction.TestRestriction) {
			if strings.HasPrefix(sourceTreePath, sourceTreeRestriction.SourceTree.Path) {
				return true, nil
			}
		}
	}
	return false, nil
}
