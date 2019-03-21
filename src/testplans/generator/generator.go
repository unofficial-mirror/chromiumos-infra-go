// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"go.chromium.org/chromiumos/infra/proto/go/testplans"
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

// BuildTarget is an OS build target, such as "kevin" or "eve".
type BuildTarget string

// ReferenceDesign is an OS platform family, such as "Google_Reef".
type ReferenceDesign string

// targetBuildResult is a conglomeration of data about a build and how to test it.
type targetBuildResult struct {
	buildTarget       BuildTarget
	buildReport       testplans.BuildReport
	perTargetTestReqs testplans.PerTargetTestRequirements
}

// CreateTestPlan generates the test plan that must be run as part of a Chrome OS build.
func CreateTestPlan(
	targetTestReqs *testplans.TargetTestRequirementsCfg,
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	buildReports []*testplans.BuildReport) (*testplans.GenerateTestPlanResponse, error) {
	testPlan := &testplans.GenerateTestPlanResponse{}

	btBuildReports := make(map[BuildTarget]testplans.BuildReport)
	for _, br := range buildReports {
		btBuildReports[BuildTarget(br.BuildTarget)] = *br
	}

	refToBuildTargets := make(map[ReferenceDesign][]BuildTarget)
	for _, br := range buildReports {
		if br.ReferenceDesign != "" {
			ref := ReferenceDesign(br.ReferenceDesign)
			refToBuildTargets[ref] = append(refToBuildTargets[ref], BuildTarget(br.BuildTarget))
		}
	}

	// BuildTargets for which HW or VM testing may be skipped, due to source tree configuration.
	skippableTests, err := extractSkippableTests(sourceTreeCfg, buildReports)
	if err != nil {
		return testPlan, err
	}

	// List of builds that will actually be tested, e.g. one per reference design or build target.
	targetBuildResults := make([]targetBuildResult, 0)
perTargetTestReq:
	for _, pttr := range targetTestReqs.PerTargetTestRequirements {
		tbr, err := selectBuildForRequirements(pttr, refToBuildTargets, btBuildReports)
		if err != nil {
			return testPlan, err
		}
		if tbr == nil {
			// Occurs when there are no affected builds for this TargetTestRequirement.
			continue perTargetTestReq
		}
		targetBuildResults = append(targetBuildResults, *tbr)
	}

	// The final list of TestUnits needed for the test plan.
	testUnits, err := createTestUnits(targetBuildResults, skippableTests)
	if err != nil {
		return testPlan, err
	}
	return &testplans.GenerateTestPlanResponse{TestUnit: testUnits}, nil
}

// createTestUnits creates the final list of tests required for the GenerateTestPlanResponse.
func createTestUnits(
	targetBuildResults []targetBuildResult,
	skippableTests map[BuildTarget]map[testType]bool) ([]*testplans.TestUnit, error) {

	testUnits := make([]*testplans.TestUnit, 0)
	for _, tbr := range targetBuildResults {
		sched, err := tbr.schedulingReqs()
		if err != nil {
			return testUnits, err
		}
		pttr := tbr.perTargetTestReqs
		if pttr.GceTestCfg != nil {
			// TODO add build payload
			testUnits = append(testUnits,
				&testplans.TestUnit{
					SchedulingRequirements: &sched,
					TestCfg:                &testplans.TestUnit_GceTestCfg{GceTestCfg: pttr.GceTestCfg}})
		}
		if pttr.HwTestCfg != nil {
			if skippableTests[tbr.buildTarget][hw] {
				log.Printf("No HW testing needed for %s", tbr.buildTarget)
			} else {
				// TODO add build payload
				testUnits = append(testUnits,
					&testplans.TestUnit{
						SchedulingRequirements: &sched,
						TestCfg:                &testplans.TestUnit_HwTestCfg{HwTestCfg: pttr.HwTestCfg}})
			}
		}
		if pttr.MoblabVmTestCfg != nil {
			// TODO add build payload
			testUnits = append(testUnits,
				&testplans.TestUnit{
					SchedulingRequirements: &sched,
					TestCfg:                &testplans.TestUnit_MoblabVmTestCfg{MoblabVmTestCfg: pttr.MoblabVmTestCfg}})
		}
		if pttr.TastVmTestCfg != nil {
			// TODO add build payload
			testUnits = append(testUnits,
				&testplans.TestUnit{
					SchedulingRequirements: &sched,
					TestCfg:                &testplans.TestUnit_TastVmTestCfg{TastVmTestCfg: pttr.TastVmTestCfg}})
		}
		if pttr.VmTestCfg != nil {
			if skippableTests[tbr.buildTarget][vm] {
				log.Printf("No VM testing needed for %s", tbr.buildTarget)
			} else {
				// TODO add build payload
				testUnits = append(testUnits,
					&testplans.TestUnit{
						SchedulingRequirements: &sched,
						TestCfg:                &testplans.TestUnit_VmTestCfg{VmTestCfg: pttr.VmTestCfg}})
			}
		}
	}
	return testUnits, nil
}

// extractSkippableTests maps BuildTargets to the test types that can be skipped for those targets,
// based on source tree test restrictions.
func extractSkippableTests(sourceTreeCfg *testplans.SourceTreeTestCfg, buildReports []*testplans.BuildReport) (
	map[BuildTarget]map[testType]bool, error) {

	skippableTests := make(map[BuildTarget]map[testType]bool)
	for _, report := range buildReports {
		buildTarget := BuildTarget(report.BuildTarget)
		skippableTests[buildTarget] = make(map[testType]bool)

		disableHWTesting, err := canDisableTesting(sourceTreeCfg, report, hw)
		if err != nil {
			return skippableTests, err
		}
		skippableTests[buildTarget][hw] = disableHWTesting
		disableVMTesting, err := canDisableTesting(sourceTreeCfg, report, vm)
		if err != nil {
			return skippableTests, err
		}
		skippableTests[buildTarget][vm] = disableVMTesting
		log.Printf("For build %s, got disableHWTesting = %t, disableVMTesting = %t", report.BuildTarget, disableHWTesting, disableVMTesting)
	}
	return skippableTests, nil
}

// selectBuildForRequirements finds a build that best matches the provided PerTargetTestRequirements.
// e.g. if the requirements want a build for a Google_Reef reference design, this method will find
// a successful, non-early-terminated Google_Reef-based built target.
func selectBuildForRequirements(
	pttr *testplans.PerTargetTestRequirements,
	refToBuildTargets map[ReferenceDesign][]BuildTarget,
	buildReports map[BuildTarget]testplans.BuildReport) (*targetBuildResult, error) {

	log.Printf("Considering testing for TargetCritera %v", pttr.TargetCriteria)
	var eligibleBuildTargets []BuildTarget
	if pttr.TargetCriteria.GetReferenceDesign() != "" {
		eligibleBuildTargets = refToBuildTargets[ReferenceDesign(pttr.TargetCriteria.GetReferenceDesign())]
	} else if pttr.TargetCriteria.GetBuildTarget() != "" {
		eligibleBuildTargets = []BuildTarget{BuildTarget(pttr.TargetCriteria.GetBuildTarget())}
	} else {
		return nil, errors.New("found a PerTargetTestRequirement without a build target or reference design")
	}
	bt, err := pickBuilderToTest(eligibleBuildTargets, buildReports)
	if err != nil {
		// Expected when a necessary builder failed, and thus we cannot continue with testing.
		return nil, err
	}
	if bt == nil {
		// There are no builds for this reference design, so this PerTargetTestRequirement is
		// irrelevant. Continue on to the next one.
		// This happens when no build was relevant due to an EarlyTerminationStatus.
		return nil, nil
	}
	return &targetBuildResult{
			buildReport:       buildReports[*bt],
			buildTarget:       BuildTarget(buildReports[*bt].BuildTarget),
			perTargetTestReqs: *pttr},
		nil
}

// schedulingReqs translates TargetCriteria into SchedulingRequirements.
func (tbr targetBuildResult) schedulingReqs() (testplans.SchedulingRequirements, error) {
	if tbr.perTargetTestReqs.TargetCriteria.GetReferenceDesign() != "" {
		return testplans.SchedulingRequirements{TargetType: &testplans.SchedulingRequirements_ReferenceDesign{
			ReferenceDesign: tbr.perTargetTestReqs.TargetCriteria.GetReferenceDesign()}}, nil
	} else if tbr.perTargetTestReqs.TargetCriteria.GetBuildTarget() != "" {
		return testplans.SchedulingRequirements{TargetType: &testplans.SchedulingRequirements_BuildTarget{
			BuildTarget: tbr.perTargetTestReqs.TargetCriteria.GetBuildTarget()}}, nil
	} else {
		return testplans.SchedulingRequirements{}, fmt.Errorf("No TargetCritera for %v", tbr)
	}
}

// pickBuilderToTest returns up to one BuildTarget that should be tested, out of the provided slice
// of BuildTargets. The returned BuildTarget, if present, is guaranteed to be one with a BuildResult.
func pickBuilderToTest(buildTargets []BuildTarget, btBuildReports map[BuildTarget]testplans.BuildReport) (*BuildTarget, error) {
	// Relevant results are those builds that weren't terminated early.
	// Early termination is a good thing. It just means that the build wasn't affected by the relevant commits.
	relevantReports := make(map[BuildTarget]testplans.BuildReport)
	for _, bt := range buildTargets {
		br, found := btBuildReports[bt]
		if !found {
			log.Printf("No build found for BuildTarget %s", bt)
			continue
		}
		if br.EarlyTerminationStatus != testplans.BuildReport_NOT_TERMINATED_EARLY &&
			br.EarlyTerminationStatus != testplans.BuildReport_EARLY_TERMINATION_STATUS_UNSPECIFIED {
			log.Printf("Disregarding %s because its EarlyTerminationStatus is %v", br.BuildTarget, br.EarlyTerminationStatus)
			continue
		}
		relevantReports[bt] = br
	}
	if len(relevantReports) == 0 {
		// None of the builds were relevant, so none of these BuildTargets needs testing.
		return nil, nil
	}
	for _, bt := range buildTargets {
		// Find and return the first relevant, successful build.
		result, found := relevantReports[bt]
		if found && result.BuildResultStatus == testplans.BuildReport_SUCCESS {
			return &bt, nil
		}
	}
	return nil, fmt.Errorf("Can't test for build target(s) %v because all builders failed", buildTargets)
}

// canDisableTesting determines whether a particular testing type is unnecessary for a given
// builder, based on source tree test restrictions.
func canDisableTesting(sourceTreeCfg *testplans.SourceTreeTestCfg, buildResult *testplans.BuildReport, tt testType) (bool, error) {
	fileCount := 0
	for _, commit := range buildResult.Commit {
		for _, file := range commit.File {
			fileCount++
			disableTesting, err := canDisableTestingForPath(file.SourceTreePath, sourceTreeCfg, tt)
			if err != nil {
				return false, err
			}
			if !disableTesting {
				log.Printf("Can't disable %s testing due to file %s", tt, file.SourceTreePath)
				return false, nil
			}
		}
	}
	// Either testing is disabled for all of the files or there are zero files.
	log.Printf("%s testing is not needed for %d files in the %s build", tt, fileCount, buildResult.BuildTarget)
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
