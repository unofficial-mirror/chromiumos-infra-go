// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package generator

import (
	"errors"
	"go.chromium.org/luci/lucicfg/external/crostesting/proto/config"
	"testplans/protos"
)

func CreateTestPlan(targetTestReqs *config.TargetTestRequirementsCfg) (*protos.GenerateTestPlanResponse, error) {
	testPlan := &protos.GenerateTestPlanResponse{}
	// Convert the TargetTestRequirementsCfg into an incomplete version of the TestPlan. This is
	// in its infancy.
	for _, perTargetTestReqs := range targetTestReqs.PerTargetTestRequirements {
		schedulingRequirements := &protos.SchedulingRequirements{}
		buildCriteria := perTargetTestReqs.GetBuildCriteria()
		if buildCriteria.GetReferenceDesign() != "" {
			schedulingRequirements.TargetType =
					&protos.SchedulingRequirements_ReferenceDesign{
						ReferenceDesign: buildCriteria.GetReferenceDesign()}
		} else if buildCriteria.GetBuildTarget() != "" {
			schedulingRequirements.TargetType =
					&protos.SchedulingRequirements_BuildTarget{
						BuildTarget: buildCriteria.GetBuildTarget()}
		} else {
			return nil, errors.New("found a PerTargetTestRequirements with no TargetType")
		}
		testUnit := &protos.TestUnit{SchedulingRequirements: schedulingRequirements}
		testPlan.TestUnit = append(testPlan.TestUnit, testUnit)
	}
	return testPlan, nil
}
