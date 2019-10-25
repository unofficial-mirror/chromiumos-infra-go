// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package generator

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes/wrappers"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"log"
	"strings"
)

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
	gerritChanges []*bbproto.GerritChange,
	changeRevs *gerrit.ChangeRevData,
	repoToBranchToSrcRoot map[string]map[string]string) (*testplans.GenerateTestPlanResponse, error) {
	testPlan := &testplans.GenerateTestPlanResponse{}

	btBuildReports := make(map[BuildTarget]bbproto.Build)
	// Filter out special builds like "chromite-cq" that don't have build targets.
	filteredBbBuilds := make([]*bbproto.Build, 0)
	for _, bb := range unfilteredBbBuilds {
		bt := getBuildTarget(bb)
		if len(bt) == 0 {
			log.Printf("filtering out build without a build target: %s", bb.GetBuilder().GetBuilder())
		} else if isPointlessBuild(bb) {
			log.Printf("filtering out because marked as pointless: %s", bb.GetBuilder().GetBuilder())
		} else if !hasTestArtifacts(bb) {
			log.Printf("filtering out with missing test artifacts: %s", bb.GetBuilder().GetBuilder())
		} else if strings.Contains(bb.GetBuilder().GetBuilder(), "kernel-v") {
			// TODO(crbug.com/1016536): Find a better way, using the existence of test artifacts.
			log.Printf("filtering out because it's a kernel builder (see https://crbug.com/1016536): %s", bb.GetBuilder().GetBuilder())
		} else {
			btBuildReports[BuildTarget(bt)] = *bb
			filteredBbBuilds = append(filteredBbBuilds, bb)
		}
	}

	// For those changes, what pruning optimizations can be done?
	pruneResult, err := extractPruneResult(sourceTreeCfg, gerritChanges, changeRevs, repoToBranchToSrcRoot)
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

	return createResponse(targetBuildResults, pruneResult)
}

func isPointlessBuild(bb *bbproto.Build) bool {
	pointlessBuild, ok := bb.GetOutput().GetProperties().GetFields()["pointless_build"]
	return ok && pointlessBuild.GetBoolValue()
}

func hasTestArtifacts(b *bbproto.Build) bool {
	art, ok := b.GetOutput().GetProperties().GetFields()["artifacts"]
	if !ok {
		return false
	}
	fba, ok := art.GetStructValue().GetFields()["files_by_artifact"]
	if !ok {
		return false
	}

	// The presence of any one of these artifacts is enough to tell us that this
	// build should be considered for testing.
	testArtifacts := []string{
		"AUTOTEST_FILES",
		"IMAGE_ZIP",
		"PINNED_GUEST_IMAGES",
		"TAST_FILES",
		"TEST_UPDATE_PAYLOAD",
	}
	fileToArtifact := fba.GetStructValue().GetFields()
	for _, ta := range testArtifacts {
		if _, ok := fileToArtifact[ta]; ok {
			return true
		}
	}
	return false
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
	pruneResult *testPruneResult) (*testplans.GenerateTestPlanResponse, error) {

	resp := &testplans.GenerateTestPlanResponse{}
targetLoop:
	for _, tbr := range targetBuildResults {
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
		filesByArtifact, ok := art.GetStructValue().Fields["files_by_artifact"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts.files_by_artifact property for build_target %s", tbr.buildTarget)
		}
		bp := &testplans.BuildPayload{
			ArtifactsGsBucket: gsBucket.GetStringValue(),
			ArtifactsGsPath:   gsPath.GetStringValue(),
			FilesByArtifact:   filesByArtifact.GetStructValue(),
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
		if pttr.HwTestCfg != nil {
			if pruneResult.disableHWTests {
				log.Printf("No HW testing needed for %s", tbr.buildTarget)
			} else if pruneResult.canSkipForOnlyTestRule(tbr.buildTarget) {
				log.Printf("Using OnlyTest rule to skip HW testing for %s", tbr.buildTarget)
			} else {
				if pruneResult.disableNonTastTests {
					log.Printf("Pruning non-Tast HW tests for %s", tbr.buildTarget)
					tastTests := make([]*testplans.HwTestCfg_HwTest, 0)
					for _, t := range pttr.HwTestCfg.HwTest {
						if t.HwTestSuiteType == testplans.HwTestCfg_TAST {
							tastTests = append(tastTests, t)
						}
					}
					pttr.HwTestCfg.HwTest = tastTests
				}
				if len(pttr.HwTestCfg.HwTest) != 0 {
					for _, hw := range pttr.HwTestCfg.HwTest {
						hw.Common = withCritical(hw.Common, critical)
					}
					resp.HwTestUnits = append(resp.HwTestUnits, &testplans.HwTestUnit{
						Common:    tuc,
						HwTestCfg: pttr.HwTestCfg})
				}
			}
		}
		if pttr.MoblabVmTestCfg != nil {
			if pruneResult.disableNonTastTests {
				log.Printf("Pruning moblab tests for %s due to non-Tast rule", tbr.buildTarget)
			} else {
				for _, moblab := range pttr.MoblabVmTestCfg.MoblabTest {
					moblab.Common = withCritical(moblab.Common, critical)
				}
				resp.MoblabVmTestUnits = append(resp.MoblabVmTestUnits, &testplans.MoblabVmTestUnit{
					Common:          tuc,
					MoblabVmTestCfg: pttr.MoblabVmTestCfg})
			}
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
			if pruneResult.disableVMTests {
				log.Printf("No VM testing needed for %s", tbr.buildTarget)
			} else if pruneResult.disableNonTastTests {
				log.Printf("Pruning non-Tast VM tests for %s due to non-Tast rule", tbr.buildTarget)
			} else {
				for _, vm := range pttr.VmTestCfg.VmTest {
					// TODO(crbug.com/1017807): make autotest VM tests critical again once
					// Alex's fix is confirmed.
					vm.Common = withCritical(vm.Common, &wrappers.BoolValue{Value: false})
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
