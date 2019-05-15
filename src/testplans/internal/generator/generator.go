// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"go.chromium.org/chromiumos/infra/proto/go/chromiumos"
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

// BuildTarget is an OS build target, such as "kevin" or "eve".
type BuildTarget string

// targetBuildResult is a conglomeration of data about a build and how to test it.
type targetBuildResult struct {
	buildTarget       BuildTarget
	buildReport       bbproto.Build
	perTargetTestReqs testplans.PerTargetTestRequirements
}

// CreateTestPlan generates the test plan that must be run as part of a Chrome OS build.
func CreateTestPlan(
	targetTestReqs *testplans.TargetTestRequirementsCfg,
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	unfilteredBbBuilds []*bbproto.Build,
	changeRevs *git.ChangeRevData,
	repoToSrcRoot map[string]string) (*testplans.GenerateTestPlanResponse, error) {
	testPlan := &testplans.GenerateTestPlanResponse{}

	btBuildReports := make(map[BuildTarget]bbproto.Build)
	// Filter out special builds like "chromite-cq" that don't have build targets.
	filteredBbBuilds := make([]*bbproto.Build, 0)
	for _, bb := range unfilteredBbBuilds {
		bt := getBuildTarget(bb)
		if len(bt) == 0 {
			log.Printf("filtering out build without a build target:\n%s",
				proto.MarshalTextString(bb.Builder))
		} else {
			btBuildReports[BuildTarget(bt)] = *bb
			filteredBbBuilds = append(filteredBbBuilds, bb)
		}
	}

	// BuildTargets for which HW or VM testing may be skipped, due to source tree configuration.
	skippableTests, err := extractSkippableTests(sourceTreeCfg, filteredBbBuilds, changeRevs, repoToSrcRoot)
	if err != nil {
		return testPlan, err
	}

	// List of builds that will actually be tested, e.g. one per build target.
	targetBuildResults := make([]targetBuildResult, 0)
perTargetTestReq:
	for _, pttr := range targetTestReqs.PerTargetTestRequirements {
		tbr, err := selectBuildForRequirements(pttr, btBuildReports)
		if err != nil {
			return testPlan, err
		}
		if tbr == nil {
			// Occurs when there are no affected builds for this TargetTestRequirement.
			continue perTargetTestReq
		}
		targetBuildResults = append(targetBuildResults, *tbr)
	}

	return createResponse(targetBuildResults, skippableTests)
}

// getBuildTarget returns the build target from the given build, or empty string if none is found.
func getBuildTarget(bb *bbproto.Build) string {
	btStruct, ok := bb.Output.Properties.Fields["build_target"]
	if !ok {
		return ""
	}
	bt, ok := btStruct.GetStructValue().Fields["name"]
	if !ok {
		return ""
	}
	return bt.GetStringValue()
}

// createResponse creates the final GenerateTestPlanResponse.
func createResponse(
	targetBuildResults []targetBuildResult,
	skippableTests map[BuildTarget]map[testType]bool) (*testplans.GenerateTestPlanResponse, error) {

	resp := &testplans.GenerateTestPlanResponse{}
targetLoop:
	for _, tbr := range targetBuildResults {
		pointlessBuild, ok := tbr.buildReport.Output.Properties.Fields["pointless_build"]
		if ok && pointlessBuild.GetBoolValue() {
			// Build terminated early and successfully. No need to test it.
			log.Printf("Skipping build %s because it's marked as pointless", tbr.buildTarget)
			continue targetLoop
		}
		art, ok := tbr.buildReport.Output.Properties.Fields["artifacts"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts output property for build_target %s", tbr.buildTarget)
		}
		gsBucket, ok := art.GetStructValue().Fields["gs_bucket"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts.gs_bucket property for build_target %s", tbr.buildTarget)
		}
		gsPath, ok := art.GetStructValue().Fields["gs_path"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts.gs_path property for build_target %s", tbr.buildTarget)
		}
		bp := &testplans.BuildPayload{
			ArtifactsGsBucket: gsBucket.GetStringValue(),
			ArtifactsGsPath:   gsPath.GetStringValue(),
		}
		pttr := tbr.perTargetTestReqs
		bt := chromiumos.BuildTarget{Name: string(tbr.buildTarget)}
		tuc := &testplans.TestUnitCommon{BuildTarget: &bt, BuildPayload: bp}
		isCritical := tbr.buildReport.Critical != bbproto.Trinary_NO
		if !isCritical {
			log.Printf("Build target %s is not critical. Skipping...", tbr.buildTarget)
			continue targetLoop
		}
		critical := &wrappers.BoolValue{Value: isCritical}
		if pttr.GceTestCfg != nil {
			for _, gce := range pttr.GceTestCfg.GceTest {
				gce.Common = withCritical(gce.Common, critical)
			}
			resp.GceTestUnits = append(resp.GceTestUnits, &testplans.GceTestUnit{
				Common:     tuc,
				GceTestCfg: pttr.GceTestCfg})
		}
		if pttr.HwTestCfg != nil {
			if skippableTests[tbr.buildTarget][hw] {
				log.Printf("No HW testing needed for %s", tbr.buildTarget)
			} else {
				for _, hw := range pttr.HwTestCfg.HwTest {
					hw.Common = withCritical(hw.Common, critical)
				}
				resp.HwTestUnits = append(resp.HwTestUnits, &testplans.HwTestUnit{
					Common:    tuc,
					HwTestCfg: pttr.HwTestCfg})
			}
		}
		if pttr.MoblabVmTestCfg != nil {
			for _, moblab := range pttr.MoblabVmTestCfg.MoblabTest {
				moblab.Common = withCritical(moblab.Common, critical)
			}
			resp.MoblabVmTestUnits = append(resp.MoblabVmTestUnits, &testplans.MoblabVmTestUnit{
				Common:          tuc,
				MoblabVmTestCfg: pttr.MoblabVmTestCfg})
		}
		if pttr.TastVmTestCfg != nil {
			for _, tastVm := range pttr.TastVmTestCfg.TastVmTest {
				tastVm.Common = withCritical(tastVm.Common, critical)
			}
			resp.TastVmTestUnits = append(resp.TastVmTestUnits, &testplans.TastVmTestUnit{
				Common:        tuc,
				TastVmTestCfg: pttr.TastVmTestCfg})
		}
		if pttr.VmTestCfg != nil {
			if skippableTests[tbr.buildTarget][vm] {
				log.Printf("No VM testing needed for %s", tbr.buildTarget)
			} else {
				for _, vm := range pttr.VmTestCfg.VmTest {
					vm.Common = withCritical(vm.Common, critical)
				}
				resp.VmTestUnits = append(resp.VmTestUnits, &testplans.VmTestUnit{
					Common:    tuc,
					VmTestCfg: pttr.VmTestCfg})
			}
		}
	}
	return resp, nil
}

func withCritical(tsc *testplans.TestSuiteCommon, critical *wrappers.BoolValue) *testplans.TestSuiteCommon {
	if tsc == nil {
		tsc = &testplans.TestSuiteCommon{}
	}
	tsc.Critical = critical
	if !critical.Value {
		log.Printf("Marking %s as not critical", tsc.DisplayName)
	}
	return tsc
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

// selectBuildForRequirements finds a build that best matches the provided PerTargetTestRequirements.
// e.g. if the requirements want a build for a reef build target, this method will find a successful,
// non-early-terminated build.
func selectBuildForRequirements(
	pttr *testplans.PerTargetTestRequirements,
	buildReports map[BuildTarget]bbproto.Build) (*targetBuildResult, error) {

	log.Printf("Considering testing for TargetCritera %v", pttr.TargetCriteria)
	var eligibleBuildTargets []BuildTarget
	if pttr.TargetCriteria.GetBuildTarget() == "" {
		return nil, errors.New("found a PerTargetTestRequirement without a build target")
	}
	eligibleBuildTargets = []BuildTarget{BuildTarget(pttr.TargetCriteria.GetBuildTarget())}
	bt, err := pickBuilderToTest(eligibleBuildTargets, buildReports)
	if err != nil {
		// Expected when a necessary builder failed, and thus we cannot continue with testing.
		return nil, err
	}
	if bt == nil {
		// There are no builds for these test criteria, so this PerTargetTestRequirement is
		// irrelevant. Continue on to the next one.
		// This happens when no build was relevant due to an EarlyTerminationStatus.
		return nil, nil
	}
	br := buildReports[*bt]
	return &targetBuildResult{
			buildReport:       br,
			buildTarget:       BuildTarget(getBuildTarget(&br)),
			perTargetTestReqs: *pttr},
		nil
}

// pickBuilderToTest returns up to one BuildTarget that should be tested, out of the provided slice
// of BuildTargets. The returned BuildTarget, if present, is guaranteed to be one with a BuildResult.
func pickBuilderToTest(buildTargets []BuildTarget, btBuildReports map[BuildTarget]bbproto.Build) (*BuildTarget, error) {
	// Relevant results are those builds that weren't terminated early.
	// Early termination is a good thing. It just means that the build wasn't affected by the relevant commits.
	relevantReports := make(map[BuildTarget]bbproto.Build)
	for _, bt := range buildTargets {
		br, found := btBuildReports[bt]
		if !found {
			log.Printf("No build found for BuildTarget %s", bt)
			continue
		}

		// TODO: Handle early termination case. As of 2019-04-05, it's not defined yet how the builder
		// will report that it terminated early.
		// Below is what the logic might look like:
		//
		//if br.EarlyTerminationStatus != testplans.BuildReport_NOT_TERMINATED_EARLY &&
		//	br.EarlyTerminationStatus != testplans.BuildReport_EARLY_TERMINATION_STATUS_UNSPECIFIED {
		//	log.Printf("Disregarding %s because its EarlyTerminationStatus is %v", br.BuildTarget, br.EarlyTerminationStatus)
		//	continue
		//}
		relevantReports[bt] = br
	}
	if len(relevantReports) == 0 {
		// None of the builds were relevant, so none of these BuildTargets needs testing.
		return nil, nil
	}
	for _, bt := range buildTargets {
		// Find and return the first relevant, successful build.
		result, found := relevantReports[bt]
		if found && result.Status == bbproto.Status_SUCCESS {
			return &bt, nil
		}
	}
	log.Printf("can't test for build target(s) %v because all builders failed\n", buildTargets)
	return nil, nil
}

// canDisableTesting determines whether a particular testing type is unnecessary for a given
// builder, based on source tree test restrictions.
func canDisableTesting(
	sourceTreeCfg *testplans.SourceTreeTestCfg,
	buildResult *bbproto.Build,
	changeRevs *git.ChangeRevData,
	repoToSrcRoot map[string]string,
	tt testType) (bool, error) {
	if len(buildResult.Input.GerritChanges) == 0 && buildResult.Input.GitilesCommit != nil {
		// In this case, the build is being performed at a particular commit, rather than for a range
		// of unmerged Gerrit CLs. The disabling of testing in this method is only applicable in the
		// Gerrit CLs case.
		log.Printf("Found a Gitiles-based build for %s, so no tests will be skipped", getBuildTarget(buildResult))
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
