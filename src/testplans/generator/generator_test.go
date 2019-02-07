// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"github.com/google/go-cmp/cmp"
	"go.chromium.org/luci/lucicfg/external/crostesting/proto/config"
	"testing"
	"testplans/protos"
)

func TestCreateCombinedTestPlan_success(t *testing.T) {
	testReqs := &config.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*config.PerTargetTestRequirements{
			{BuildCriteria: &config.BuildCriteria{
				TargetType: &config.BuildCriteria_ReferenceDesign{ReferenceDesign: "Google_Reef"}}},
			{BuildCriteria: &config.BuildCriteria{
				TargetType: &config.BuildCriteria_BuildTarget{BuildTarget: "kevin"}}},
		},
	}

	actualTestPlan, _ := CreateCombinedTestPlan(testReqs)

	expectedTestPlan := &protos.CombinedTestPlan{
		TestPlan: []*protos.TestPlan{
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_ReferenceDesign{
					ReferenceDesign: "Google_Reef"}}},
			{SchedulingRequirements: &protos.SchedulingRequirements{
				TargetType: &protos.SchedulingRequirements_BuildTarget{
					BuildTarget: "kevin"}}},
		},
	}

	if diff := cmp.Diff(expectedTestPlan, actualTestPlan); diff != "" {
		t.Errorf("CreateCombinedTestPlan bad result (-want/+got)\n%s", diff)
	}
}

func TestCreateCombinedTestPlan_inputMissingTargetType(t *testing.T) {
	testReqs := &config.TargetTestRequirementsCfg{
		PerTargetTestRequirements: []*config.PerTargetTestRequirements{
			// This is missing a TargetType.
			{BuildCriteria: &config.BuildCriteria{}},
			{BuildCriteria: &config.BuildCriteria{
				TargetType: &config.BuildCriteria_BuildTarget{BuildTarget: "kevin"}}},
		},
	}
	if _, err := CreateCombinedTestPlan(testReqs); err == nil {
		t.Errorf("Expected an error to be returned")
	}
}
