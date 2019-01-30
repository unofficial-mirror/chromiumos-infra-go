// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package executors_test

import (
	"context"
	"reflect"
	"testing"

	"testplans/common/exec"
	model "testplans/common/protos"
	"testplans/test_executor/internal/executors"

	"github.com/golang/protobuf/jsonpb"
)

func testPlan() *model.CombinedTestPlan {
	s := `
	{
		"test_plan": [
			{
				"test_env": "hw",
				"test_suite": "bvt-inline",
				"scheduling_requirements": {
					"reference_design": "reef"
				},
				"build_payload": {
					"image": [
						{
							"image_name": "pyroR6544"
						}
					]
				}
			}
		]
	}
`

	tp := &model.CombinedTestPlan{}
	_ = jsonpb.UnmarshalString(s, tp)

	return tp
}

func TestExecuteHWGroup(t *testing.T) {
	tp := testPlan()

	mc := exec.MockCommandContext()
	defer mc.Close()

	err := executors.Execute(context.Background(), tp)
	if err != nil {

	}

	if mc.Name() != "skylab" {
		t.Errorf("Incorrect cmd name:\nexpected: %s\nactual: %s", "skylab", mc.Name())
	}

	expected := []string{
		"-board", "reef",
		"-pool", "paladin",
		"-image", "pyroR6544",
		"bvt-inline",
	}
	actual := mc.Args()

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Skylab called with incorrect args:\nexpected: %v\nactual: %v", expected, actual)
	}
}
