// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"flag"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/lucicfg/external/crostesting/proto/config"
	"io/ioutil"
	"log"
	"testplans/plans"
)

var (
	sourceTreeConfigPath       = flag.String("source_tree_config_path", "", "Path to source_tree_test_config.cfg")
	targetTestRequirementsPath = flag.String("target_test_requirements_path", "", "Path to target_test_requirements.cfg")
)

func main() {
	flag.Parse()
	// Read the SourceTreeConfig JSON file into a proto.
	sourceTreeConfigBytes, err := ioutil.ReadFile(*sourceTreeConfigPath)
	if err != nil {
		log.Fatalf("Failed reading source_tree_config_path\n%v", err)
	}
	sourceTreeTestConfig := &config.SourceTreeTestCfg{}
	if err := jsonpb.UnmarshalString(string(sourceTreeConfigBytes), sourceTreeTestConfig);
			err != nil {
		log.Fatalf("Couldn't decode %s as a SourceTreeTestCfg\n%v", sourceTreeConfigPath, err)
	}
	log.Printf("Read SourceTreeTestCfg:\n%s", proto.MarshalTextString(sourceTreeTestConfig))

	// Read the TargetTestRequirements JSON file into a proto.
	targetTestRequirementsBytes, err := ioutil.ReadFile(*targetTestRequirementsPath)
	if err != nil {
		log.Fatalf("Failed reading target_test_requirements_path\n%s", err)
	}
	targetTestRequirements := &config.TargetTestRequirementsCfg{}
	if err := jsonpb.UnmarshalString(string(targetTestRequirementsBytes), targetTestRequirements);
			err != nil {
		log.Fatalf(
			"Couldn't decode %s as a TargetTestRequirementsCfg\n%s",
			targetTestRequirementsPath, err)
	}
	log.Printf(
		"Read TargetTestRequirementsCfg:\n%s", proto.MarshalTextString(targetTestRequirements))

	combinedTestPlan, err := plans.CreateCombinedTestPlan(targetTestRequirements)
	if err != nil {
		log.Fatalf("Error creating CombinedTestPlan:\n%v", err)
	}

	log.Printf(
		"Resultant combined test plan:\n%s", proto.MarshalTextString(combinedTestPlan))
}
