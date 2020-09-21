// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package generator

import (
	"fmt"
	"github.com/bmatcuk/doublestar"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	"log"
)

type testType int

type testGroup string

const (
	hw testType = iota
	vm
	nonTast
)

var (
	testTypeFilter = map[testType]func(testReqs *testplans.TestRestriction) bool{
		hw:      func(testReqs *testplans.TestRestriction) bool { return testReqs.DisableHwTests },
		vm:      func(testReqs *testplans.TestRestriction) bool { return testReqs.DisableVmTests },
		nonTast: func(testReqs *testplans.TestRestriction) bool { return testReqs.DisableNonTastTests },
	}
)

func (tt testType) String() string {
	return [...]string{"Hw", "Vm"}[tt]
}

type testPruneResult struct {
	disableHWTests      bool
	disableVMTests      bool
	disableNonTastTests bool
	onlyTestGroups      map[testGroup]bool
	oneofTestGroups     map[testGroup]bool
	alsoTestGroups      map[testGroup]bool
}

func (tpr testPruneResult) hasOneofOrOnlyTestRules() bool {
	return len(tpr.oneofTestGroups) > 0 || len(tpr.onlyTestGroups) > 0
}

func (tpr testPruneResult) canSkipForOnlyTestRule(groups []*testplans.TestSuiteCommon_TestSuiteGroup) bool {
	// If the source config didn't specify any onlyTestGroups, we can't skip testing for the groups in the params.
	if len(tpr.onlyTestGroups) == 0 {
		return false
	}
	for _, g := range groups {
		if tpr.onlyTestGroups[testGroup(g.TestSuiteGroup)] {
			return false
		}
	}
	return true
}

func (tpr testPruneResult) mustAddForAlsoTestRule(groups []*testplans.TestSuiteCommon_TestSuiteGroup) bool {
	for _, g := range groups {
		if tpr.alsoTestGroups[testGroup(g.TestSuiteGroup)] {
			return true
		}
	}
	return false
}

func extractPruneResult(
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	srcPaths []string) (*testPruneResult, error) {

	result := &testPruneResult{}

	if len(srcPaths) == 0 {
		// This happens during postsubmit runs, for example.
		log.Print("no gerrit_changes, so no tests will be skipped")
		return result, nil
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
	disableNonTastTests := true
	for _, fileSrcPath := range srcPaths {
		if disableNonTastTests {
			disableNonTastTestsForPath, err := canDisableTestingForPath(fileSrcPath, sourceTreeCfg, nonTast)
			if err != nil {
				return result, err
			}
			if !disableNonTastTestsForPath {
				log.Printf("cannot disable non-Tast testing due to file %s", fileSrcPath)
				disableNonTastTests = false
			}
		}
	}

	canOnlyTestSomeBuilders := true
	onlyTestGroups := make(map[testGroup]bool)
	oneofTestGroups := make(map[testGroup]bool)
	for _, fileSrcPath := range srcPaths {
		if canOnlyTestSomeBuilders {
			fileOnlyTestGroups, err := getOnlyTestGroups(fileSrcPath, sourceTreeCfg)
			if err != nil {
				return result, err
			}
			fileOneofTestGroups, err := getOneofTestGroups(fileSrcPath, sourceTreeCfg)
			if err != nil {
				return result, err
			}
			if len(fileOnlyTestGroups) == 0 && len(fileOneofTestGroups) == 0 {
				log.Printf("cannot limit set of builders for testing due to %s", fileSrcPath)
				canOnlyTestSomeBuilders = false
				onlyTestGroups = make(map[testGroup]bool)
			} else {
				for g, include := range fileOnlyTestGroups {
					onlyTestGroups[g] = include
				}
				for g, include := range fileOneofTestGroups {
					oneofTestGroups[g] = include
				}
			}
		}
	}

	alsoTestGroups := make(map[testGroup]bool)
	for _, fileSrcPath := range srcPaths {
		fileAlsoTestGroups, err := getAlsoTestGroups(fileSrcPath, sourceTreeCfg)
		if err != nil {
			return result, err
		}
		for k, v := range fileAlsoTestGroups {
			alsoTestGroups[k] = v
			log.Printf("will also test testGroup %v due to file %v", k, fileSrcPath)
		}
	}

	return &testPruneResult{
			disableHWTests:      disableHW,
			disableVMTests:      disableVM,
			disableNonTastTests: disableNonTastTests,
			onlyTestGroups:      onlyTestGroups,
			oneofTestGroups:     oneofTestGroups,
			alsoTestGroups:      alsoTestGroups},
		nil
}

func getOnlyTestGroups(
	sourcePath string,
	sourceTreeCfg *testplans.SourceTreeTestCfg) (map[testGroup]bool, error) {
	onlyTestGroups := make(map[testGroup]bool)
	for _, sourceTreeRestriction := range sourceTreeCfg.SourceTreeTestRestriction {
		match, err := doublestar.Match(sourceTreeRestriction.GetFilePattern().GetPattern(), sourcePath)
		if err != nil {
			return onlyTestGroups, err
		}
		if match && sourceTreeRestriction.TestRestriction.GetCqOnlyTestGroup() != "" {
			onlyTestGroups[testGroup(sourceTreeRestriction.TestRestriction.GetCqOnlyTestGroup())] = true
		}
	}
	return onlyTestGroups, nil
}

// getOneofTestGroups extracts rules from config about any type of oneof testing
// that can be done for the provided path. For each of the keys in the returned
// map, at least one test suite must be tested.
func getOneofTestGroups(
	sourcePath string,
	sourceTreeCfg *testplans.SourceTreeTestCfg) (map[testGroup]bool, error) {
	oneofTestGroups := make(map[testGroup]bool)
	for _, sourceTreeRestriction := range sourceTreeCfg.SourceTreeTestRestriction {
		match, err := doublestar.Match(sourceTreeRestriction.GetFilePattern().GetPattern(), sourcePath)
		if err != nil {
			return oneofTestGroups, err
		}
		otg := sourceTreeRestriction.TestRestriction.GetCqOneofTestGroups()
		if match && otg != nil && len(otg.GetName()) != 0 {
			for _, g := range otg.GetName() {
				oneofTestGroups[testGroup(g)] = true
			}
		}
	}
	return oneofTestGroups, nil
}

func getAlsoTestGroups(
	sourcePath string,
	sourceTreeCfg *testplans.SourceTreeTestCfg) (map[testGroup]bool, error) {
	alsoTestGroups := make(map[testGroup]bool)
	for _, sourceTreeRestriction := range sourceTreeCfg.SourceTreeTestRestriction {
		match, err := doublestar.Match(sourceTreeRestriction.GetFilePattern().GetPattern(), sourcePath)
		if err != nil {
			return alsoTestGroups, err
		}
		if match && sourceTreeRestriction.TestRestriction.GetCqAlsoTestGroup() != "" {
			alsoTestGroups[testGroup(sourceTreeRestriction.TestRestriction.GetCqAlsoTestGroup())] = true
		}
	}
	return alsoTestGroups, nil
}

// canDisableTestingForPath determines whether a particular testing type is unnecessary for
// a given file, based on source tree test restrictions.
func canDisableTestingForPath(sourcePath string, sourceTreeCfg *testplans.SourceTreeTestCfg, tt testType) (bool, error) {
	for _, sourceTreeRestriction := range sourceTreeCfg.SourceTreeTestRestriction {
		testFilter, ok := testTypeFilter[tt]
		if !ok {
			return false, fmt.Errorf("Missing test filter for %v", tt)
		}
		if testFilter(sourceTreeRestriction.TestRestriction) {
			match, err := doublestar.Match(sourceTreeRestriction.GetFilePattern().GetPattern(), sourcePath)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		}
	}
	return false, nil
}
