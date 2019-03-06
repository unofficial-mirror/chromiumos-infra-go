// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"testing"
	"testplans/protos"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/luci/lucicfg/external/crostesting/proto/config"
)

func TestCreateCombinedTestPlan_success(t *testing.T) {
	reefGceTestCfg := &config.GceTestCfg{GceTest: []*config.GceTestCfg_GceTest{
		{TestType: "GCE reef"},
	}}
	reefMoblabVmTestCfg := &config.MoblabVmTestCfg{MoblabTest: []*config.MoblabVmTestCfg_MoblabTest{
		{TestType: "Moblab reef"},
	}}
	kevinHWTestCfg := &config.HwTestCfg{HwTest: []*config.HwTestCfg_HwTest{
		{Suite: "HW kevin"},
	}}
	kevinTastVMTestCfg := &config.TastVmTestCfg{TastVmTest: []*config.TastVmTestCfg_TastVmTest{
		{SuiteName: "Tast kevin"},
	}}
	kevinVMTestCfg := &config.VmTestCfg{VmTest: []*config.VmTestCfg_VmTest{
		{TestType: "VM kevin"},
	}}
	testReqs := &config.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*config.PerTargetTestRequirements{
			{TargetCriteria: &config.TargetCriteria{
				TargetType: &config.TargetCriteria_ReferenceDesign{ReferenceDesign: "Google_Reef"}},
				GceTestCfg:      reefGceTestCfg,
				MoblabVmTestCfg: reefMoblabVmTestCfg},
			{TargetCriteria: &config.TargetCriteria{
				TargetType: &config.TargetCriteria_BuildTarget{BuildTarget: "kevin"}},
				HwTestCfg:     kevinHWTestCfg,
				TastVmTestCfg: kevinTastVMTestCfg,
				VmTestCfg:     kevinVMTestCfg},
		},
	}
	sourceTreeTestCfg := &config.SourceTreeTestCfg{
		SourceTreeTestRestriction: []*config.SourceTreeTestRestriction{
			{SourceTree: &config.SourceTree{Path: "hw/tests/not/needed/here"},
				TestRestriction: &config.TestRestriction{DisableHwTests: true}}}}
	buildReports := []*protos.BuildReport{
		{BuildTarget: "kevin",
			BuildResultStatus:      protos.BuildReport_SUCCESS,
			EarlyTerminationStatus: protos.BuildReport_NOT_TERMINATED_EARLY,
			Commit:                 []*protos.Commit{{File: []*protos.File{{SourceTreePath: "a/b/c"}}}}},
		{BuildTarget: "some reef build target",
			ReferenceDesign:        "Google_Reef",
			BuildResultStatus:      protos.BuildReport_SUCCESS,
			EarlyTerminationStatus: protos.BuildReport_NOT_TERMINATED_EARLY,
			Commit:                 []*protos.Commit{{File: []*protos.File{{SourceTreePath: "c/d/e"}}}}},
	}

	actualTestPlan, err := CreateTestPlan(testReqs, sourceTreeTestCfg, buildReports)
	if err != nil {
		t.Error(err)
	}

	expectedTestPlan := &protos.GenerateTestPlanResponse{
		TestUnit: []*protos.TestUnit{
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_ReferenceDesign{
					ReferenceDesign: "Google_Reef"}},
				TestCfg: &protos.TestUnit_GceTestCfg{GceTestCfg: reefGceTestCfg}},
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_ReferenceDesign{
					ReferenceDesign: "Google_Reef"}},
				TestCfg: &protos.TestUnit_MoblabVmTestCfg{MoblabVmTestCfg: reefMoblabVmTestCfg}},
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &protos.TestUnit_HwTestCfg{HwTestCfg: kevinHWTestCfg}},
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &protos.TestUnit_TastVmTestCfg{TastVmTestCfg: kevinTastVMTestCfg}},
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}},
				TestCfg: &protos.TestUnit_VmTestCfg{VmTestCfg: kevinVMTestCfg}},
		}}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_inputMissingTargetType(t *testing.T) {
	testReqs := &config.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*config.PerTargetTestRequirements{
			// This is missing a TargetType.
			{TargetCriteria: &config.TargetCriteria{}},
			{TargetCriteria: &config.TargetCriteria{
				TargetType: &config.TargetCriteria_BuildTarget{BuildTarget: "kevin"}}},
		},
	}
	sourceTreeTestCfg := &config.SourceTreeTestCfg{}
	buildReports := []*protos.BuildReport{}
	if _, err := CreateTestPlan(testReqs, sourceTreeTestCfg, buildReports); err == nil {
		t.Errorf("Expected an error to be returned")
	}
}
