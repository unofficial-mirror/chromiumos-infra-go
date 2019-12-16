// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"testing"

	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

const (
	GS_BUCKET      = "gs://chromeos-image-archive"
	GS_PATH_PREFIX = "gs/path/"
)

var (
	simpleFilesByArtifactValue = _struct.Value{Kind: &_struct.Value_StructValue{StructValue: &_struct.Struct{
		Fields: map[string]*_struct.Value{
			"AUTOTEST_FILES": {Kind: &_struct.Value_ListValue{}},
		},
	}}}
	simpleFilesByArtifact = _struct.Struct{Fields: simpleFilesByArtifactValue.GetStructValue().Fields}
	emptyGerritChanges    []*bbproto.GerritChange
)

func makeBuildbucketBuild(buildTarget string, status bbproto.Status, critical bool) *bbproto.Build {
	var criticalVal bbproto.Trinary
	if critical {
		criticalVal = bbproto.Trinary_YES
	} else {
		criticalVal = bbproto.Trinary_NO
	}
	b := &bbproto.Build{
		Builder: &bbproto.BuilderID{Builder: buildTarget + "-cq"},
		Critical: criticalVal,
		Input:    &bbproto.Build_Input{},
		Output: &bbproto.Build_Output{
			Properties: &_struct.Struct{
				Fields: map[string]*_struct.Value{
					"build_target": {
						Kind: &_struct.Value_StructValue{StructValue: &_struct.Struct{
							Fields: map[string]*_struct.Value{
								"name": {Kind: &_struct.Value_StringValue{StringValue: buildTarget}},
							},
						}},
					},
					"artifacts": {
						Kind: &_struct.Value_StructValue{StructValue: &_struct.Struct{
							Fields: map[string]*_struct.Value{
								"gs_bucket":         {Kind: &_struct.Value_StringValue{StringValue: GS_BUCKET}},
								"gs_path":           {Kind: &_struct.Value_StringValue{StringValue: GS_PATH_PREFIX + buildTarget}},
								"files_by_artifact": &simpleFilesByArtifactValue,
							},
						}},
					},
				},
			},
		},
		Status: status,
	}
	return b
}

func TestCreateCombinedTestPlan_oneUnitSuccess(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW kevin",
			SkylabBoard:     "kev",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "hw/tests/not/needed/here/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/repo/name": {"refs/heads/master": "src/to/file"}}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, emptyGerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		HwTestUnits: []*testplans.HwTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
	}
	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_manyUnitSuccess(t *testing.T) {
	reefMoblabVmTestCfg := &testplans.MoblabVmTestCfg{MoblabTest: []*testplans.MoblabVmTestCfg_MoblabTest{
		{TestType: "Moblab reef", Common: &testplans.TestSuiteCommon{Critical: &wrappers.BoolValue{Value: true}}},
	}}
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:       "HW kevin",
			SkylabBoard: "kev",
		},
	}}
	kevinTastVMTestCfg := &testplans.TastVmTestCfg{TastVmTest: []*testplans.TastVmTestCfg_TastVmTest{
		{SuiteName: "Tast kevin"},
	}}
	kevinVMTestCfg := &testplans.VmTestCfg{VmTest: []*testplans.VmTestCfg_VmTest{
		{TestSuite: "VM kevin"},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "reef"}},
				MoblabVmTestCfg: reefMoblabVmTestCfg},
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg:     kevinHWTestCfg,
				TastVmTestCfg: kevinTastVMTestCfg,
				VmTestCfg:     kevinVMTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "hw/tests/not/needed/here/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true),
		makeBuildbucketBuild("reef", bbproto.Status_SUCCESS, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/repo/name",
			Files:   []string{"a/b/c"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/repo/name": {"refs/heads/master": "src/to/file"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2}}
	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		MoblabVmTestUnits: []*testplans.MoblabVmTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "reef",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "reef"}},
				MoblabVmTestCfg: reefMoblabVmTestCfg},
		},
		HwTestUnits: []*testplans.HwTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
		TastVmTestUnits: []*testplans.TastVmTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "kevin"}},
				TastVmTestCfg: kevinTastVMTestCfg},
		},
		VmTestUnits: []*testplans.VmTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "kevin"}},
				VmTestCfg: kevinVMTestCfg},
		}}
	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_successDespiteOneFailedBuilder(t *testing.T) {
	// In this test, the kevin builder failed, so the output test plan will not contain a test unit
	// for kevin.

	reefHwTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{SkylabBoard: "some reef",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST},
	}}
	kevinVMTestCfg := &testplans.VmTestCfg{VmTest: []*testplans.VmTestCfg_VmTest{
		{TestSuite: "VM kevin"},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "reef"}},
				HwTestCfg: reefHwTestCfg},
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				VmTestCfg: kevinVMTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "hw/tests/not/needed/here/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_FAILURE, true),
		makeBuildbucketBuild("reef", bbproto.Status_SUCCESS, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/repo/name",
			Files:   []string{"a/b/c"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/repo/name": {"refs/heads/master": "src/to/file"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		HwTestUnits: []*testplans.HwTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "reef",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "reef"}},
				HwTestCfg: reefHwTestCfg},
		}}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_skipsUnnecessaryHardwareTest(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW kevin",
			SkylabBoard:     "kev",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "no/hw/tests/here/some/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/test/repo/name",
			Files:   []string{"some/file"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/test/repo/name": {"refs/heads/master": "no/hw/tests/here"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_doesOnlyTest(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW kevin",
			SkylabBoard:     "kev",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	bobHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW bob",
			SkylabBoard:     "bob board",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "bob"}},
				HwTestCfg: bobHWTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "no/hw/tests/here/some/**"},
				TestRestriction: &testplans.TestRestriction{
					OnlyTestBuildTargets: []*chromiumos.BuildTarget{
						{Name: "kevin"}},
				}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true),
		makeBuildbucketBuild("bob", bbproto.Status_SUCCESS, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/test/repo/name",
			Files:   []string{"some/file"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/test/repo/name": {"refs/heads/master": "no/hw/tests/here"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		HwTestUnits: []*testplans.HwTestUnit{
			{Common: &testplans.TestUnitCommon{
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
					FilesByArtifact:   &simpleFilesByArtifact,
				},
				BuildTarget: &chromiumos.BuildTarget{Name: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
	}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_inputMissingTargetType(t *testing.T) {
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			// This is missing a TargetType.
			{TargetCriteria: &testplans.TargetCriteria{}},
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}}},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{}
	bbBuilds := []*bbproto.Build{}
	if _, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, emptyGerritChanges, &gerrit.ChangeRevData{}, map[string]map[string]string{}); err == nil {
		t.Errorf("Expected an error to be returned")
	}
}

func TestCreateCombinedTestPlan_skipsPointlessBuild(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW kevin",
			SkylabBoard:     "kev",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "hw/tests/not/needed/here/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuild := makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true)
	bbBuild.Output.Properties.Fields["pointless_build"] = &_struct.Value{Kind: &_struct.Value_BoolValue{BoolValue: true}}
	bbBuilds := []*bbproto.Build{bbBuild}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/repo/name",
			Files:   []string{"a/b/c"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/repo/name": {"refs/heads/master": "src/to/file"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateTestPlan_succeedsOnNoBuildTarget(t *testing.T) {
	testReqs := &testplans.TargetTestRequirementsCfg{}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{}
	bbBuilds := []*bbproto.Build{
		// build target is empty.
		makeBuildbucketBuild("", bbproto.Status_FAILURE, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{})
	repoToBranchToSrcRoot := map[string]map[string]string{}

	_, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, emptyGerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Errorf("expected no error, but got %v", err)
	}
}

func TestCreateCombinedTestPlan_skipsNonCritical(t *testing.T) {
	// In this test, the build is not critical, so no test unit will be produced.

	reefHwTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{SkylabBoard: "my reef",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "reef"}},
				HwTestCfg: reefHwTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "hw/tests/not/needed/here/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("reef", bbproto.Status_SUCCESS, false),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/repo/name",
			Files:   []string{"a/b/c"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/repo/name": {"refs/heads/master": "src/to/file"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		HwTestUnits: []*testplans.HwTestUnit{}}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_ignoresNonArtifactBuild(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW kevin",
			SkylabBoard:     "kev",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg: kevinHWTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "hw/tests/not/needed/here/**"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	build := makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true)

	// Remove the AUTOTEST_FILES files_by_artifact key, thus making this whole
	// build unusable for testing.
	delete(
		build.GetOutput().GetProperties().GetFields()["artifacts"].GetStructValue().GetFields()["files_by_artifact"].GetStructValue().GetFields(),
		"AUTOTEST_FILES")
	bbBuilds := []*bbproto.Build{build}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/repo/name": {"refs/heads/master": "src/to/file"}}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, emptyGerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{}
	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_skipsNonTastTest(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{
			Suite:           "HW kevin",
			SkylabBoard:     "kev",
			HwTestSuiteType: testplans.HwTestCfg_AUTOTEST,
		},
	}}
	kevinVmTestCfg := &testplans.VmTestCfg{VmTest: []*testplans.VmTestCfg_VmTest{
		{
			TestSuite: "some sweet VM suite",
		},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg: kevinHWTestCfg,
				VmTestCfg: kevinVmTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{FilePattern: &testplans.FilePattern{Pattern: "no/tast/tests/here/some/**"},
				TestRestriction: &testplans.TestRestriction{DisableNonTastTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, true),
	}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/test/repo/name",
			Files:   []string{"some/file"},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{"chromiumos/test/repo/name": {"refs/heads/master": "no/tast/tests/here"}}
	gerritChanges := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, gerritChanges, chRevData, repoToBranchToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}
