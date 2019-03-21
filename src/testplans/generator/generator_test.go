// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
)

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
				TargetType: &testplans.TargetCriteria_ReferenceDesign{ReferenceDesign: "Google_Reef"}},
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
	buildReports := []*testplans.BuildReport{
		{BuildTarget: "kevin",
			BuildResultStatus:      testplans.BuildReport_SUCCESS,
			EarlyTerminationStatus: testplans.BuildReport_NOT_TERMINATED_EARLY,
			Commit:                 []*testplans.Commit{{File: []*testplans.File{{SourceTreePath: "a/b/c"}}}}},
		{BuildTarget: "some reef build target",
			ReferenceDesign:        "Google_Reef",
			BuildResultStatus:      testplans.BuildReport_SUCCESS,
			EarlyTerminationStatus: testplans.BuildReport_NOT_TERMINATED_EARLY,
			Commit:                 []*testplans.Commit{{File: []*testplans.File{{SourceTreePath: "c/d/e"}}}}},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, buildReports)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &testplans.GenerateTestPlanResponse{
		TestUnit: []*testplans.TestUnit{
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_ReferenceDesign{
					ReferenceDesign: "Google_Reef"}},
				TestCfg: &testplans.TestUnit_GceTestCfg{GceTestCfg: reefGceTestCfg}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_ReferenceDesign{
					ReferenceDesign: "Google_Reef"}},
				TestCfg: &testplans.TestUnit_MoblabVmTestCfg{MoblabVmTestCfg: reefMoblabVmTestCfg}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &testplans.TestUnit_HwTestCfg{HwTestCfg: kevinHWTestCfg}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &testplans.TestUnit_TastVmTestCfg{TastVmTestCfg: kevinTastVMTestCfg}},
			{SchedulingRequirements: &testplans.SchedulingRequirements{
				TargetType: &testplans.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &testplans.TestUnit_VmTestCfg{VmTestCfg: kevinVMTestCfg}},
		}}

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
	buildReports := []*testplans.BuildReport{}
	if _, err := CreateTestPlan(testReqs, sourceTreeTestCfg, buildReports); err == nil {
		t.Errorf("Expected an error to be returned")
	}
}
