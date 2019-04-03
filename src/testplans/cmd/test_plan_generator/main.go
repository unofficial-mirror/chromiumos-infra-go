// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"context"
	"flag"
	"github.com/maruel/subcommands"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"io/ioutil"
	"log"
	"os"
	"testplans/generator"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
)

func cmdGenTestPlan(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "gen-test-plan --input_json=/path/to/input.json --output_json=/path/to/output.json",
		ShortDesc: "Generates a test plan",
		LongDesc:  "Generates a test plan",
		CommandRun: func() subcommands.CommandRun {
			c := &getTestPlanRun{}
			c.authOpts = authOpts
			c.Flags.StringVar(&c.inputJson, "input_json", "", "Path to JSON proto representing a GenerateTestPlanRequest")
			c.Flags.StringVar(&c.outputJson, "output_json", "", "Path to file to write output GenerateTestPlanResponse JSON proto")
			return c
		}}
}

type getTestPlanRun struct {
	subcommands.CommandRunBase
	authOpts   auth.Options
	inputJson  string
	outputJson string
}

func (c *getTestPlanRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	flag.Parse()

	inputBytes, err := ioutil.ReadFile(c.inputJson)
	if err != nil {
		log.Printf("Failed reading input_json\n%v", err)
		return 1
	}
	req := &testplans.GenerateTestPlanRequest{}
	if err := jsonpb.Unmarshal(bytes.NewReader(inputBytes), req); err != nil {
		log.Printf("Couldn't decode %s as a GenerateTestPlanRequest\n%v", c.inputJson, err)
		return 2
	}

	// Read the SourceTreeConfig JSON file into a proto.
	sourceTreeBytes, err := ioutil.ReadFile(req.SourceTreeConfigPath)
	if err != nil {
		log.Printf("Failed reading source_tree_config_path\n%v", err)
		return 3
	}
	sourceTreeConfig := &testplans.SourceTreeTestCfg{}
	if err := jsonpb.Unmarshal(bytes.NewReader(sourceTreeBytes), sourceTreeConfig); err != nil {
		log.Printf("Couldn't decode %s as a SourceTreeTestCfg\n%v", req.SourceTreeConfigPath, err)
		return 4
	}
	log.Printf("Read SourceTreeTestCfg:\n%s", proto.MarshalTextString(sourceTreeConfig))

	// Read the TargetTestRequirements JSON file into a proto.
	testReqsBytes, err := ioutil.ReadFile(req.TargetTestRequirementsPath)
	if err != nil {
		log.Printf("Failed reading target_test_requirements_path\n%s", err)
		return 5
	}
	testReqsConfig := &testplans.TargetTestRequirementsCfg{}
	if err := jsonpb.Unmarshal(bytes.NewReader(testReqsBytes), testReqsConfig); err != nil {
		log.Printf(
			"Couldn't decode %s as a TargetTestRequirementsCfg\n%s",
			req.TargetTestRequirementsPath, err)
		return 6
	}
	log.Printf(
		"Read TargetTestRequirementsCfg:\n%s", proto.MarshalTextString(testReqsConfig))

	buildReports := make([]*testplans.BuildReport, 0)
	for _, brPath := range req.BuildReportPath {
		buildReportBytes, err := ioutil.ReadFile(brPath.FilePath)
		if err != nil {
			log.Printf("Failed reading build_report_path\n%v", err)
			return 7
		}
		buildReport := &testplans.BuildReport{}
		if err := jsonpb.Unmarshal(bytes.NewReader(buildReportBytes), buildReport); err != nil {
			log.Printf("Couldn't decode %s as a BuildReport\n%v", req.BuildReportPath, err)
			return 8
		}
		buildReports = append(buildReports, buildReport)
	}

	testPlan, err := generator.CreateTestPlan(testReqsConfig, sourceTreeConfig, buildReports)
	if err != nil {
		log.Printf("Error creating test plan:\n%v", err)
		return 9
	}

	marshal := &jsonpb.Marshaler{EmitDefaults: true, Indent: "  "}
	jsonOutput, err := marshal.MarshalToString(testPlan)
	if err != nil {
		log.Printf("Failed to marshal %v\n%v", testPlan, err)
		return 10
	}
	if err = ioutil.WriteFile(c.outputJson, []byte(jsonOutput), 0644); err != nil {
		log.Printf("Failed to write output JSON!\n%v", err)
		return 11
	}
	log.Printf("Wrote output to %s", c.outputJson)
	return 0
}

func GetApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name: "test_planner",

		Context: func(ctx context.Context) context.Context {
			return ctx
		},

		Commands: []*subcommands.Command{
			authcli.SubcommandInfo(defaultAuthOpts, "auth-info", false),
			authcli.SubcommandLogin(defaultAuthOpts, "auth-login", false),
			authcli.SubcommandLogout(defaultAuthOpts, "auth-logout", false),
			cmdGenTestPlan(defaultAuthOpts),
		},
	}
}

func main() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{gerrit.OAuthScope, auth.OAuthScopeEmail}
	app := GetApplication(opts)
	os.Exit(subcommands.Run(app, nil))
}
