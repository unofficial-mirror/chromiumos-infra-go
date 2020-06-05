// Copyright 2019 The Chromium OS Authors. All rights reserved.
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
	build             bbproto.Build
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

	buildResults := eligibleTestBuilds(unfilteredBbBuilds)

	// All the files in the GerritChanges, in source tree form.
	srcPaths, err := srcPaths(gerritChanges, changeRevs, repoToBranchToSrcRoot)
	if err != nil {
		return testPlan, err
	}

	// For those changes, what pruning optimizations can be done?
	pruneResult, err := extractPruneResult(sourceTreeCfg, srcPaths)
	if err != nil {
		return testPlan, err
	}

	// List of builds that will actually be tested, e.g. one per builder name.
	targetBuildResults := make([]buildResult, 0)
perTargetTestReq:
	for _, pttr := range targetTestReqs.PerTargetTestRequirements {
		tbr, err := selectBuildForRequirements(pttr, buildResults)
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

func eligibleTestBuilds(unfilteredBbBuilds []*bbproto.Build) map[buildId]*bbproto.Build {
	buildIdToBuild := make(map[buildId]*bbproto.Build)
	for _, bb := range unfilteredBbBuilds {
		bt := getBuildTarget(bb)
		if len(bt) == 0 {
			log.Printf("filtering out build without a build target: %s", bb.GetBuilder().GetBuilder())
		} else if isPointlessBuild(bb) {
			log.Printf("filtering out because marked as pointless: %s", bb.GetBuilder().GetBuilder())
		} else if !hasTestArtifacts(bb) {
			log.Printf("filtering out with missing test artifacts: %s", bb.GetBuilder().GetBuilder())
		} else {
			buildIdToBuild[buildId{buildTarget: bt, builderName: bb.GetBuilder().GetBuilder()}] = bb
		}
	}
	return buildIdToBuild
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
	// loop over the merged (Buildbucket build, TargetTestRequirements).
	for _, tbr := range targetBuildResults {
		art, ok := tbr.build.Output.Properties.Fields["artifacts"]
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
		criticalBuild := tbr.build.Critical != bbproto.Trinary_NO
		if !criticalBuild {
			// We formerly didn't test noncritical builders, but now we do.
			// See https://crbug.com/1040602.
			log.Printf("Builder %s is not critical, but we can still test it.", tbr.buildId.builderName)
		}

		if pttr.HwTestCfg != nil {
			hwTestUnit := getHwTestUnit(tuc, pttr.HwTestCfg.HwTest, pruneResult, criticalBuild)
			if hwTestUnit != nil {
				resp.HwTestUnits = append(resp.HwTestUnits, hwTestUnit)
			}
		}

		if pttr.MoblabVmTestCfg != nil {
			moblabTestUnit := getMoblabTestUnit(tuc, pttr.MoblabVmTestCfg.MoblabTest, pruneResult, criticalBuild)
			if moblabTestUnit != nil {
				resp.MoblabVmTestUnits = append(resp.MoblabVmTestUnits, moblabTestUnit)
			}
		}

		if pttr.TastVmTestCfg != nil {
			tastVmTestUnit := getTastVmTestUnit(tuc, pttr.TastVmTestCfg.TastVmTest, pruneResult, criticalBuild)
			if tastVmTestUnit != nil {
				resp.TastVmTestUnits = append(resp.TastVmTestUnits, tastVmTestUnit)
			}
		}
		if pttr.DirectTastVmTestCfg != nil {
			directTastVmTestUnit := getTastVmTestUnit(tuc, pttr.DirectTastVmTestCfg.TastVmTest, pruneResult, criticalBuild)
			if directTastVmTestUnit != nil {
				resp.DirectTastVmTestUnits = append(resp.DirectTastVmTestUnits, directTastVmTestUnit)
			}
		}
		if pttr.VmTestCfg != nil {
			vmTestUnit := getVmTestUnit(tuc, pttr.VmTestCfg.VmTest, pruneResult, criticalBuild)
			if vmTestUnit != nil {
				resp.VmTestUnits = append(resp.VmTestUnits, vmTestUnit)
			}
		}
	}
	return resp, nil
}

func getHwTestUnit(tuc *testplans.TestUnitCommon, tests []*testplans.HwTestCfg_HwTest, pruneResult *testPruneResult, criticalBuild bool) *testplans.HwTestUnit {
	if tests == nil {
		return nil
	}
	tu := &testplans.HwTestUnit{
		Common:    tuc,
		HwTestCfg: &testplans.HwTestCfg{},
	}
testLoop:
	for _, t := range tests {
		if pruneResult.disableHWTests {
			log.Printf("no HW testing needed for %v", t.Common.DisplayName)
			continue testLoop
		}
		if pruneResult.disableNonTastTests && t.HwTestSuiteType != testplans.HwTestCfg_TAST {
			log.Printf("skipping non-Tast testing for %v", t.Common.DisplayName)
			continue testLoop
		}
		mustTest := pruneResult.mustAddForAlsoTestRule(t.Common.TestSuiteGroups)
		if !mustTest {
			if pruneResult.canSkipForOnlyTestRule(t.Common.TestSuiteGroups) {
				log.Printf("using OnlyTest rule to skip HW testing for %v", t.Common.DisplayName)
				continue testLoop
			}
			if t.Common.DisableByDefault {
				log.Printf("%v is disabled by default, and it was not triggered to be enabled", t.Common.DisplayName)
				continue testLoop
			}
		}
		log.Printf("adding testing for %v", t.Common.DisplayName)
		t.Common = withCritical(t.Common, criticalBuild)
		tu.HwTestCfg.HwTest = append(tu.HwTestCfg.HwTest, t)
	}
	if len(tu.HwTestCfg.HwTest) > 0 {
		return tu
	}
	return nil
}

func getMoblabTestUnit(tuc *testplans.TestUnitCommon, tests []*testplans.MoblabVmTestCfg_MoblabTest, pruneResult *testPruneResult, criticalBuild bool) *testplans.MoblabVmTestUnit {
	if tests == nil {
		return nil
	}
	tu := &testplans.MoblabVmTestUnit{
		Common:          tuc,
		MoblabVmTestCfg: &testplans.MoblabVmTestCfg{},
	}
testLoop:
	for _, t := range tests {
		if pruneResult.disableNonTastTests {
			log.Printf("skipping non-Tast testing for %v", t.Common.DisplayName)
			continue testLoop
		}
		mustTest := pruneResult.mustAddForAlsoTestRule(t.Common.TestSuiteGroups)
		if !mustTest {
			if pruneResult.canSkipForOnlyTestRule(t.Common.TestSuiteGroups) {
				log.Printf("using OnlyTest rule to skip HW testing for %v", t.Common.DisplayName)
				continue testLoop
			}
			if t.Common.DisableByDefault {
				log.Printf("%v is disabled by default, and it was not triggered to be enabled", t.Common.DisplayName)
				continue testLoop
			}
		}
		log.Printf("adding testing for %v", t.Common.DisplayName)
		t.Common = withCritical(t.Common, criticalBuild)
		tu.MoblabVmTestCfg.MoblabTest = append(tu.MoblabVmTestCfg.MoblabTest, t)
	}
	if len(tu.MoblabVmTestCfg.MoblabTest) > 0 {
		return tu
	}
	return nil
}

func getTastVmTestUnit(tuc *testplans.TestUnitCommon, tests []*testplans.TastVmTestCfg_TastVmTest, pruneResult *testPruneResult, criticalBuild bool) *testplans.TastVmTestUnit {
	if tests == nil {
		return nil
	}
	tu := &testplans.TastVmTestUnit{
		Common:        tuc,
		TastVmTestCfg: &testplans.TastVmTestCfg{},
	}
testLoop:
	for _, t := range tests {
		if pruneResult.disableVMTests {
			log.Printf("no Tast VM testing needed for %v", t.Common.DisplayName)
			continue testLoop
		}
		mustTest := pruneResult.mustAddForAlsoTestRule(t.Common.TestSuiteGroups)
		if !mustTest {
			if pruneResult.canSkipForOnlyTestRule(t.Common.TestSuiteGroups) {
				log.Printf("using OnlyTest rule to skip Tast VM testing for %v", t.Common.DisplayName)
				continue testLoop
			}
			if t.Common.DisableByDefault {
				log.Printf("%v is disabled by default, and it was not triggered to be enabled", t.Common.DisplayName)
				continue testLoop
			}
		}
		log.Printf("adding testing for %v", t.Common.DisplayName)
		t.Common = withCritical(t.Common, criticalBuild)
		tu.TastVmTestCfg.TastVmTest = append(tu.TastVmTestCfg.TastVmTest, t)
	}
	if len(tu.TastVmTestCfg.TastVmTest) > 0 {
		return tu
	}
	return nil
}

func getVmTestUnit(tuc *testplans.TestUnitCommon, tests []*testplans.VmTestCfg_VmTest, pruneResult *testPruneResult, criticalBuild bool) *testplans.VmTestUnit {
	if tests == nil {
		return nil
	}
	tu := &testplans.VmTestUnit{
		Common:    tuc,
		VmTestCfg: &testplans.VmTestCfg{},
	}
testLoop:
	for _, t := range tests {
		if pruneResult.disableVMTests {
			log.Printf("no VM testing needed for %v", t.Common.DisplayName)
			continue testLoop
		}
		if pruneResult.disableNonTastTests {
			log.Printf("skipping non-Tast testing for %v", t.Common.DisplayName)
			continue testLoop
		}
		mustTest := pruneResult.mustAddForAlsoTestRule(t.Common.TestSuiteGroups)
		if !mustTest {
			if pruneResult.canSkipForOnlyTestRule(t.Common.TestSuiteGroups) {
				log.Printf("using OnlyTest rule to skip VM testing for %v", t.Common.DisplayName)
				continue testLoop
			}
			if t.Common.DisableByDefault {
				log.Printf("%v is disabled by default, and it was not triggered to be enabled", t.Common.DisplayName)
				continue testLoop
			}
		}
		log.Printf("adding testing for %v", t.Common.DisplayName)
		t.Common = withCritical(t.Common, criticalBuild)
		tu.VmTestCfg.VmTest = append(tu.VmTestCfg.VmTest, t)
	}
	if len(tu.VmTestCfg.VmTest) > 0 {
		return tu
	}
	return nil
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
	buildIdToBuild map[buildId]*bbproto.Build) (*buildResult, error) {

	log.Printf("Considering testing for TargetCritera %v", pttr.TargetCriteria)
	if pttr.TargetCriteria.GetBuildTarget() == "" {
		return nil, errors.New("found a PerTargetTestRequirement without a build target")
	}
	eligibleBuildIds := []buildId{buildId{pttr.TargetCriteria.GetBuildTarget(), pttr.TargetCriteria.GetBuilderName()}}
	bt, err := pickBuilderToTest(eligibleBuildIds, buildIdToBuild)
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
	br := buildIdToBuild[*bt]
	return &buildResult{
			build:             *br,
			buildId:           buildId{buildTarget: getBuildTarget(br), builderName: br.GetBuilder().GetBuilder()},
			perTargetTestReqs: *pttr},
		nil
}

// pickBuilderToTest returns up to one buildId that should be tested, out of the provided slice
// of buildIds. The returned buildId, if present, is guaranteed to be one with a BuildResult.
func pickBuilderToTest(buildIds []buildId, buildIdToBuild map[buildId]*bbproto.Build) (*buildId, error) {
	// Relevant results are those builds that weren't terminated early.
	// Early termination is a good thing. It just means that the build wasn't affected by the relevant commits.
	relevantReports := make(map[buildId]*bbproto.Build)
	for _, bt := range buildIds {
		br, found := buildIdToBuild[bt]
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
