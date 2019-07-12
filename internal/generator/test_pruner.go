// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package generator

import (
	"fmt"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"log"
	"strings"
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

type testPruneResult struct {
	disableHWTests       bool
	disableVMTests       bool
	onlyTestBuildTargets map[BuildTarget]bool
}

// canSkipForOnlyTestRule identifies whether testing for a provided build target
// can be skipped due to the only-test rules. e.g. if we only need to test on
// "reef", this will return false for all non-reef build targets.
func (tpr testPruneResult) canSkipForOnlyTestRule(bt BuildTarget) bool {
	// If no only-test build targets were specified, we can't skip testing for
	// any build targets by only-test rules.
	if len(tpr.onlyTestBuildTargets) == 0 {
		return false
	}
	isAnOnlyTestTarget, _ := tpr.onlyTestBuildTargets[bt]
	return !isAnOnlyTestTarget
}

func extractPruneResult(
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	changes []*bbproto.GerritChange,
	changeRevs *gerrit.ChangeRevData,
	repoToBranchToSrcRoot map[string]map[string]string) (*testPruneResult, error) {

	result := &testPruneResult{}

	if len(changes) == 0 {
		// This happens during postsubmit runs, for example.
		log.Print("no gerrit_changes, so no tests will be skipped")
		return result, nil
	}

	// All the files in the GerritChanges, in source tree form.
	srcPaths, err := srcPaths(changes, changeRevs, repoToBranchToSrcRoot)
	if err != nil {
		return result, err
	}

	disableHW := true
	for _, fileSrcPath := range srcPaths {
		if disableHW {
			disableHWForPath, err := canDisableTestingForPath(fileSrcPath, sourceTreeCfg, hw)
			if err != nil {
				return result, err
			}
			if !disableHWForPath {
				log.Printf("cannot disable HW testing due to file %s", fileSrcPath)
				disableHW = false
			}
		}
	}
	disableVM := true
	for _, fileSrcPath := range srcPaths {
		if disableVM {
			disableVMForPath, err := canDisableTestingForPath(fileSrcPath, sourceTreeCfg, vm)
			if err != nil {
				return result, err
			}
			if !disableVMForPath {
				log.Printf("cannot disable VM testing due to file %s", fileSrcPath)
				disableVM = false
			}
		}
	}
	canOnlyTestSomeBuildTargets := true
	onlyTestBuildTargets := make(map[BuildTarget]bool)
	for _, fileSrcPath := range srcPaths {
		if canOnlyTestSomeBuildTargets {
			fileOnlyTestBuildTargets, err := checkOnlyTestBuildTargets(fileSrcPath, sourceTreeCfg)
			if err != nil {
				return result, err
			}
			if len(fileOnlyTestBuildTargets) == 0 {
				log.Printf("cannot limit set of build targets for testing due to %s", fileSrcPath)
				canOnlyTestSomeBuildTargets = false
				onlyTestBuildTargets = make(map[BuildTarget]bool)
			} else {
				for k, v := range fileOnlyTestBuildTargets {
					onlyTestBuildTargets[k] = v
				}
			}
		}
	}
	return &testPruneResult{
			disableHWTests:       disableHW,
			disableVMTests:       disableVM,
			onlyTestBuildTargets: onlyTestBuildTargets},
		nil
}

// srcPaths extracts the source paths from each of the provided Gerrit changes.
func srcPaths(
	changes []*bbproto.GerritChange,
	changeRevs *gerrit.ChangeRevData,
	repoToBranchToSrcRoot map[string]map[string]string) ([]string, error) {
	srcPaths := make([]string, 0)
	for _, commit := range changes {
		chRev, err := changeRevs.GetChangeRev(commit.Host, commit.Change, int32(commit.Patchset))
		if err != nil {
			return srcPaths, err
		}
		for _, file := range chRev.Files {
			branchMapping, found := repoToBranchToSrcRoot[chRev.Project]
			if !found {
				return srcPaths, fmt.Errorf("Found no branch mapping for project %s", chRev.Project)
			}
			srcRootMapping, found := branchMapping[chRev.Branch]
			if !found {
				return srcPaths, fmt.Errorf("Found no source mapping for project %s and branch %s", chRev.Project, chRev.Branch)
			}
			srcPaths = append(srcPaths, fmt.Sprintf("%s/%s", srcRootMapping, file))
		}
	}
	return srcPaths, nil
}

// checkOnlyTestBuildTargets checks if the provided path is covered by an
// only-test rule, which would allow us to exclude testing for all other
// build targets.
func checkOnlyTestBuildTargets(
	sourceTreePath string,
	sourceTreeCfg *testplans.SourceTreeTestCfg) (map[BuildTarget]bool, error) {
	result := make(map[BuildTarget]bool)
	for _, sourceTreeRestriction := range sourceTreeCfg.SourceTreeTestRestriction {
		if hasPathPrefix(sourceTreePath, sourceTreeRestriction.SourceTree.Path) {
			for _, otbt := range sourceTreeRestriction.TestRestriction.OnlyTestBuildTargets {
				result[BuildTarget(otbt.Name)] = true
			}
		}
	}
	return result, nil
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
			if hasPathPrefix(sourceTreePath, sourceTreeRestriction.SourceTree.Path) {
				return true, nil
			}
		}
	}
	return false, nil
}

// hasPathPrefix checks if the provided string has a provided path prefix.
// e.g. ab/cd/ef, ab --> true
//      ab/cd, ab/c --> false
func hasPathPrefix(s string, prefix string) bool {
	if s == prefix {
		return true
	}
	prefixAsDir := strings.TrimSuffix(prefix, "/") + "/"
	return strings.HasPrefix(s, prefixAsDir)
}
