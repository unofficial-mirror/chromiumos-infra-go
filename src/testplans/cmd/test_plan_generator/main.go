// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"testplans/generator"
	"testplans/protos"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/lucicfg/external/crostesting/proto/config"
)

var (
	inputJson  = flag.String("input_json", "", "Path to JSON proto representing a GenerateTestPlanRequest")
	outputJson = flag.String("output_json", "", "Path to file to write output GenerateTestPlanResponse JSON proto")
)

func main() {
	flag.Parse()

	inputBytes, err := ioutil.ReadFile(*inputJson)
	if err != nil {
		log.Fatalf("Failed reading input_json\n%v", err)
	}
	req := &protos.GenerateTestPlanRequest{}
	if err := jsonpb.Unmarshal(bytes.NewReader(inputBytes), req); err != nil {
		log.Fatalf("Couldn't decode %s as a GenerateTestPlanRequest\n%v", *inputJson, err)
	}

	// Read the SourceTreeConfig JSON file into a proto.
	sourceTreeBytes, err := ioutil.ReadFile(req.SourceTreeConfigPath)
	if err != nil {
		log.Fatalf("Failed reading source_tree_config_path\n%v", err)
	}
	sourceTreeConfig := &config.SourceTreeTestCfg{}
	if err := jsonpb.Unmarshal(bytes.NewReader(sourceTreeBytes), sourceTreeConfig); err != nil {
		log.Fatalf("Couldn't decode %s as a SourceTreeTestCfg\n%v", req.SourceTreeConfigPath, err)
	}
	log.Printf("Read SourceTreeTestCfg:\n%s", proto.MarshalTextString(sourceTreeConfig))

	// Read the TargetTestRequirements JSON file into a proto.
	testReqsBytes, err := ioutil.ReadFile(req.TargetTestRequirementsPath)
	if err != nil {
		log.Fatalf("Failed reading target_test_requirements_path\n%s", err)
	}
	testReqsConfig := &config.TargetTestRequirementsCfg{}
	if err := jsonpb.Unmarshal(bytes.NewReader(testReqsBytes), testReqsConfig); err != nil {
		log.Fatalf(
			"Couldn't decode %s as a TargetTestRequirementsCfg\n%s",
			req.TargetTestRequirementsPath, err)
	}
	log.Printf(
		"Read TargetTestRequirementsCfg:\n%s", proto.MarshalTextString(testReqsConfig))

	buildReports := make([]*protos.BuildReport, 0)
	for _, brPath := range req.BuildReportPath {
		buildReportBytes, err := ioutil.ReadFile(brPath.FilePath)
		if err != nil {
			log.Fatalf("Failed reading build_report_path\n%v", err)
		}
		buildReport := &protos.BuildReport{}
		if err := jsonpb.Unmarshal(bytes.NewReader(buildReportBytes), buildReport); err != nil {
			log.Fatalf("Couldn't decode %s as a BuildReport\n%v", req.BuildReportPath, err)
		}
		buildReports = append(buildReports, buildReport)
	}

	testPlan, err := generator.CreateTestPlan(testReqsConfig, sourceTreeConfig, buildReports)
	if err != nil {
		log.Fatalf("Error creating test plan:\n%v", err)
	}

	marshal := &jsonpb.Marshaler{EmitDefaults: true, Indent: "  "}
	jsonOutput, err := marshal.MarshalToString(testPlan)
	if err != nil {
		log.Fatalf("Failed to marshal %v\n%v", testPlan, err)
	}
	if err = ioutil.WriteFile(*outputJson, []byte(jsonOutput), 0644); err != nil {
		log.Fatalf("Failed to write output JSON!\n%v", err)
	}
	log.Printf("Wrote output to %s", *outputJson)
}
