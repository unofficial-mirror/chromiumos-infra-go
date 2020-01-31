// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package generator

import (
	"errors"
	"fmt"
	"log"

	"github.com/golang/protobuf/ptypes/wrappers"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

type buildId struct {
	buildTarget string
	builderName string
}

// buildResult is a conglomeration of data about a build and how to test it.
type buildResult struct {
	buildId           buildId
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

	btBuildReports := make(map[buildId]bbproto.Build)
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
		} else {
			btBuildReports[buildId{buildTarget: bt, builderName: bb.GetBuilder().GetBuilder()}] = *bb
			filteredBbBuilds = append(filteredBbBuilds, bb)
		}
	}

	// For those changes, what pruning optimizations can be done?
	pruneResult, err := extractPruneResult(sourceTreeCfg, gerritChanges, changeRevs, repoToBranchToSrcRoot)
	if err != nil {
		return testPlan, err
	}

	// List of builds that will actually be tested, e.g. one per builder name.
	targetBuildResults := make([]buildResult, 0)
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
	targetBuildResults []buildResult,
	pruneResult *testPruneResult) (*testplans.GenerateTestPlanResponse, error) {

	resp := &testplans.GenerateTestPlanResponse{}
	for _, tbr := range targetBuildResults {
		art, ok := tbr.buildReport.Output.Properties.Fields["artifacts"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts output property for builder %s", tbr.buildId.builderName)
		}
		gsBucket, ok := art.GetStructValue().Fields["gs_bucket"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts.gs_bucket property for builder %s", tbr.buildId.builderName)
		}
		gsPath, ok := art.GetStructValue().Fields["gs_path"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts.gs_path property for builder %s", tbr.buildId.builderName)
		}
		filesByArtifact, ok := art.GetStructValue().Fields["files_by_artifact"]
		if !ok {
			return nil, fmt.Errorf("found no artifacts.files_by_artifact property for builder %s", tbr.buildId.builderName)
		}
		bp := &testplans.BuildPayload{
			ArtifactsGsBucket: gsBucket.GetStringValue(),
			ArtifactsGsPath:   gsPath.GetStringValue(),
			FilesByArtifact:   filesByArtifact.GetStructValue(),
		}
		pttr := tbr.perTargetTestReqs
		bt := chromiumos.BuildTarget{Name: tbr.buildId.buildTarget}
		tuc := &testplans.TestUnitCommon{BuildTarget: &bt, BuildPayload: bp, BuilderName: tbr.buildId.builderName}
		isBuildCritical := tbr.buildReport.Critical != bbproto.Trinary_NO
		if !isBuildCritical {
			// We formerly didn't test noncritical builders, but now we do.
			// See https://crbug.com/1040602.
			log.Printf("Builder %s is not critical, but we can still test it.", tbr.buildId.builderName)
		}
		if pttr.HwTestCfg != nil {
			if pruneResult.disableHWTests {
				log.Printf("No HW testing needed for %s", tbr.buildId.builderName)
			} else if pruneResult.canSkipForOnlyTestRule(tbr.buildId) {
				log.Printf("Using OnlyTest rule to skip HW testing for %s", tbr.buildId.builderName)
			} else {
				if pruneResult.disableNonTastTests {
					log.Printf("Pruning non-Tast HW tests for %s", tbr.buildId.builderName)
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
						hw.Common = withCritical(hw.Common, isBuildCritical)
					}
					resp.HwTestUnits = append(resp.HwTestUnits, &testplans.HwTestUnit{
						Common:    tuc,
						HwTestCfg: pttr.HwTestCfg})
				}
			}
		}
		if pttr.MoblabVmTestCfg != nil {
			if pruneResult.disableNonTastTests {
				log.Printf("Pruning moblab tests for %s due to non-Tast rule", tbr.buildId.builderName)
			} else {
				for _, moblab := range pttr.MoblabVmTestCfg.MoblabTest {
					moblab.Common = withCritical(moblab.Common, isBuildCritical)
				}
				resp.MoblabVmTestUnits = append(resp.MoblabVmTestUnits, &testplans.MoblabVmTestUnit{
					Common:          tuc,
					MoblabVmTestCfg: pttr.MoblabVmTestCfg})
			}
		}
		if pttr.TastVmTestCfg != nil {
			for _, tastVm := range pttr.TastVmTestCfg.TastVmTest {
				tastVm.Common = withCritical(tastVm.Common, isBuildCritical)
			}
			resp.TastVmTestUnits = append(resp.TastVmTestUnits, &testplans.TastVmTestUnit{
				Common:        tuc,
				TastVmTestCfg: pttr.TastVmTestCfg})
		}
		if pttr.DirectTastVmTestCfg != nil {
			for _, tastVm := range pttr.DirectTastVmTestCfg.TastVmTest {
				tastVm.Common = withCritical(tastVm.Common, isBuildCritical)
			}
			resp.DirectTastVmTestUnits = append(resp.DirectTastVmTestUnits, &testplans.TastVmTestUnit{
				Common:        tuc,
				TastVmTestCfg: pttr.DirectTastVmTestCfg})
		}
		if pttr.VmTestCfg != nil {
			if pruneResult.disableVMTests {
				log.Printf("No VM testing needed for %s", tbr.buildId.builderName)
			} else if pruneResult.disableNonTastTests {
				log.Printf("Pruning non-Tast VM tests for %s due to non-Tast rule", tbr.buildId.builderName)
			} else {
				for _, vm := range pttr.VmTestCfg.VmTest {
					vm.Common = withCritical(vm.Common, isBuildCritical)
				}
				resp.VmTestUnits = append(resp.VmTestUnits, &testplans.VmTestUnit{
					Common:    tuc,
					VmTestCfg: pttr.VmTestCfg})
			}
		}
	}
	return resp, nil
}

func withCritical(tsc *testplans.TestSuiteCommon, buildCritical bool) *testplans.TestSuiteCommon {
	if tsc == nil {
		tsc = &testplans.TestSuiteCommon{}
	}
	suiteCritical := true
	if tsc.Critical != nil {
		suiteCritical = tsc.Critical.Value
	}
	// If either the build was noncritical or the suite is configured to be
	// noncritical, then make the suite noncritical. As of now we don't even
	// schedule suites for noncritical builders, but if we ever change that logic,
	// this seems like the right way to set suite criticality.
	tsc.Critical = &wrappers.BoolValue{Value: buildCritical && suiteCritical}
	if !tsc.Critical.Value {
		log.Printf("Marking %s as not critical", tsc.DisplayName)
	}
	return tsc
}

// selectBuildForRequirements finds a build that best matches the provided PerTargetTestRequirements.
// e.g. if the requirements want a build for a reef build target, this method will find a successful,
// non-early-terminated build.
func selectBuildForRequirements(
	pttr *testplans.PerTargetTestRequirements,
	buildReports map[buildId]bbproto.Build) (*buildResult, error) {

	log.Printf("Considering testing for TargetCritera %v", pttr.TargetCriteria)
	if pttr.TargetCriteria.GetBuildTarget() == "" {
		return nil, errors.New("found a PerTargetTestRequirement without a build target")
	}
	eligibleBuildIds := []buildId{buildId{pttr.TargetCriteria.GetBuildTarget(), pttr.TargetCriteria.GetBuilderName()}}
	bt, err := pickBuilderToTest(eligibleBuildIds, buildReports)
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
	return &buildResult{
			buildReport:       br,
			buildId:           buildId{buildTarget: getBuildTarget(&br), builderName: br.GetBuilder().GetBuilder()},
			perTargetTestReqs: *pttr},
		nil
}

// pickBuilderToTest returns up to one buildId that should be tested, out of the provided slice
// of buildIds. The returned buildId, if present, is guaranteed to be one with a BuildResult.
func pickBuilderToTest(buildIds []buildId, btBuildReports map[buildId]bbproto.Build) (*buildId, error) {
	// Relevant results are those builds that weren't terminated early.
	// Early termination is a good thing. It just means that the build wasn't affected by the relevant commits.
	relevantReports := make(map[buildId]bbproto.Build)
	for _, bt := range buildIds {
		br, found := btBuildReports[bt]
		if !found {
			log.Printf("No build found for buildId %s", bt)
			continue
		}
		relevantReports[bt] = br
	}
	if len(relevantReports) == 0 {
		// None of the builds were relevant, so none of these builds needs testing.
		return nil, nil
	}
	for _, bt := range buildIds {
		// Find and return the first relevant, successful build.
		result, found := relevantReports[bt]
		if found && result.Status == bbproto.Status_SUCCESS {
			return &bt, nil
		}
	}
	log.Printf("can't test for builders %v because all builders failed\n", buildIds)
	return nil, nil
}
