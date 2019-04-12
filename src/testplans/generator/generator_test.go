// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"testing"
	"testplans/git"

	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/google/go-cmp/cmp"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

const (
	GS_BUCKET      = "gs://chromeos-image-archive"
	GS_PATH_PREFIX = "gs/path/"
)

func makeBuildbucketBuild(buildTarget string, status bbproto.Status, changes []*bbproto.GerritChange) *bbproto.Build {
	b := &bbproto.Build{
		Input: &bbproto.Build_Input{},
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
								"gs_bucket": {Kind: &_struct.Value_StringValue{StringValue: GS_BUCKET}},
								"gs_path":   {Kind: &_struct.Value_StringValue{StringValue: GS_PATH_PREFIX + buildTarget}},
							},
						}},
					},
				},
			},
		},
		Status: status,
	}
	for _, c := range changes {
		b.Input.GerritChanges = append(b.Input.GerritChanges, c)
	}
	return b
}

func TestCreateCombinedTestPlan_success(t *testing.T) {
	reefGceTestCfg := &testplans.GceTestCfg{GceTest: []*testplans.GceTestCfg_GceTest{
		{TestType: "GCE reef"},
	}}
	reefMoblabVmTestCfg := &testplans.MoblabVmTestCfg{MoblabTest: []*testplans.MoblabVmTestCfg_MoblabTest{
		{TestType: "Moblab reef"},
	}}
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{Suite: "HW kevin"},
	}}
	kevinTastVMTestCfg := &testplans.TastVmTestCfg{TastVmTest: []*testplans.TastVmTestCfg_TastVmTest{
		{SuiteName: "Tast kevin"},
	}}
	kevinVMTestCfg := &testplans.VmTestCfg{VmTest: []*testplans.VmTestCfg_VmTest{
		{TestType: "VM kevin"},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "reef"}},
				GceTestCfg:      reefGceTestCfg,
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
			{SourceTree: &testplans.SourceTree{Path: "hw/tests/not/needed/here"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, []*bbproto.GerritChange{
			{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
		}),
		makeBuildbucketBuild("reef", bbproto.Status_SUCCESS, []*bbproto.GerritChange{
			{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
		}),
	}
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Project: "chromiumos/repo/name",
			Files:   []string{"a/b/c"},
		},
	})
	repoToSrcRoot := map[string]string{"chromiumos/repo/name": "src/to/file"}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, chRevData, repoToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		TestUnit: []*testplans.TestUnit{
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "reef"}},
				TestCfg: &testplans.TestUnit_GceTestCfg{GceTestCfg: reefGceTestCfg},
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "reef",
				}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "reef"}},
				TestCfg: &testplans.TestUnit_MoblabVmTestCfg{MoblabVmTestCfg: reefMoblabVmTestCfg},
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "reef",
				}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &testplans.TestUnit_HwTestCfg{HwTestCfg: kevinHWTestCfg},
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
				}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &testplans.TestUnit_TastVmTestCfg{TastVmTestCfg: kevinTastVMTestCfg},
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
				}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &testplans.TestUnit_VmTestCfg{VmTestCfg: kevinVMTestCfg},
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "kevin",
				}},
		}}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_successDespiteOneFailedBuilder(t *testing.T) {
	// In this test, the kevin builder failed, so the output test plan will not contain a test unit
	// for kevin.

	reefGceTestCfg := &testplans.GceTestCfg{GceTest: []*testplans.GceTestCfg_GceTest{
		{TestType: "GCE reef"},
	}}
	kevinVMTestCfg := &testplans.VmTestCfg{VmTest: []*testplans.VmTestCfg_VmTest{
		{TestType: "VM kevin"},
	}}
	testReqs := &testplans.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*testplans.PerTargetTestRequirements{
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "reef"}},
				GceTestCfg: reefGceTestCfg},
			{TargetCriteria: &testplans.TargetCriteria{
				TargetType: &testplans.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				VmTestCfg: kevinVMTestCfg},
		},
	}
	sourceTreeTestCfg := &testplans.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*testplans.SourceTreeTestRestriction{
			{SourceTree: &testplans.SourceTree{Path: "hw/tests/not/needed/here"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_FAILURE, []*bbproto.GerritChange{
			{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
		}),
		makeBuildbucketBuild("reef", bbproto.Status_SUCCESS, []*bbproto.GerritChange{
			{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
		}),
	}
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Project: "chromiumos/repo/name",
			Files:   []string{"a/b/c"},
		},
	})
	repoToSrcRoot := map[string]string{"chromiumos/repo/name": "src/to/file"}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, chRevData, repoToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		TestUnit: []*testplans.TestUnit{
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "reef"}},
				TestCfg: &testplans.TestUnit_GceTestCfg{GceTestCfg: reefGceTestCfg},
				BuildPayload: &testplans.BuildPayload{
					ArtifactsGsBucket: GS_BUCKET,
					ArtifactsGsPath:   GS_PATH_PREFIX + "reef",
				}},
		}}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_skipsUnnecessaryHardwareTest(t *testing.T) {
	kevinHWTestCfg := &testplans.HwTestCfg{HwTest: []*testplans.HwTestCfg_HwTest{
		{Suite: "HW kevin"},
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
			{SourceTree: &testplans.SourceTree{Path: "no/hw/tests/here/some/file"},
				TestRestriction: &testplans.TestRestriction{DisableHwTests: true}}}}
	bbBuilds := []*bbproto.Build{
		makeBuildbucketBuild("kevin", bbproto.Status_SUCCESS, []*bbproto.GerritChange{
			{Host: "test-review.googlesource.com", Change: 123, Patchset: 2},
		}),
	}
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Project: "chromiumos/test/repo/name",
			Files:   []string{"some/file"},
		},
	})
	repoToSrcRoot := map[string]string{"chromiumos/test/repo/name": "no/hw/tests/here"}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, chRevData, repoToSrcRoot)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		TestUnit: []*testplans.TestUnit{}}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan); diff != "" {
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
	if _, err := CreateTestPlan(testReqs, sourceTreeTestCfg, bbBuilds, &git.ChangeRevData{}, map[string]string{}); err == nil {
		t.Errorf("Expected an error to be returned")
	}
}
