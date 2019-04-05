// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"context"
	"flag"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testplans/generator"
	"testplans/git"
	"testplans/repo"
)

func cmdGenTestPlan(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "gen-test-plan --input_json=/path/to/input.json --output_json=/path/to/output.json",
		ShortDesc: "Generates a test plan",
		LongDesc:  "Generates a test plan",
		CommandRun: func() subcommands.CommandRun {
			c := &getTestPlanRun{}
			c.authFlags = authcli.Flags{}
			c.authFlags.Register(c.GetFlags(), authOpts)
			c.Flags.StringVar(&c.inputJson, "input_json", "", "Path to JSON proto representing a GenerateTestPlanRequest")
			c.Flags.StringVar(&c.outputJson, "output_json", "", "Path to file to write output GenerateTestPlanResponse JSON proto")
			return c
		}}
}

type getTestPlanRun struct {
	subcommands.CommandRunBase
	authFlags  authcli.Flags
	inputJson  string
	outputJson string
}

func fetchClData(authedClient *http.Client, ctx context.Context, bbBuilds []*bbproto.Build) (*git.ChangeRevData, error) {
	changeIds := make([]git.ChangeRevKey, 0)
	for _, build := range bbBuilds {
		for _, ch := range build.Input.GerritChanges {
			changeIds = append(changeIds, git.ChangeRevKey{Host: ch.Host, ChangeNum: ch.Change, Revision: int32(ch.Patchset)})
		}
	}
	return git.GetChangeRevData(authedClient, ctx, changeIds)
}

func (c *getTestPlanRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	flag.Parse()
	// TODO(seanabraham@chromium.org): Break up this method into smaller ones.

	// Do auth needed for Gerrit RPCs.
	ctx := context.Background()
	authOpts, err := c.authFlags.Options()
	if err != nil {
		log.Print(err)
		return 16
	}
	authedClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		log.Print(err)
		return 12
	}

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

	// Run the repo tool to get a mapping from Gerrit project to source root.
	if req.ChromiumosCheckoutRoot == "" {
		log.Printf("Must set request ChromiumosCheckoutRoot")
		return 14
	}
	repoToSrcRoot, err := repo.GetRepoToSourceRoot(req.ChromiumosCheckoutRoot)
	if err != nil {
		log.Printf("Error with repo tool call\n%v", err)
		return 15
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

	// Read the Buildbucket Build protos.
	bbBuilds := make([]*bbproto.Build, 0)
	for _, bbBuildPath := range req.BuildbucketBuildPath {
		bbBuildBytes, err := ioutil.ReadFile(bbBuildPath.FilePath)
		if err != nil {
			log.Printf("Failed reading build_report_path\n%v", err)
			return 7
		}
		bbBuild := &bbproto.Build{}
		if err := jsonpb.Unmarshal(bytes.NewReader(bbBuildBytes), bbBuild); err != nil {
			log.Printf("Couldn't decode %s as a Buildbucket Build\n%v", bbBuildPath, err)
			return 8
		}
		bbBuilds = append(bbBuilds, bbBuild)
	}

	// Create an authenticated client for Gerrit RPCs, then fetch all required CL data from Gerrit.
	changeRevs, err := fetchClData(authedClient, ctx, bbBuilds)
	if err != nil {
		log.Printf("Failed to fetch CL data from Gerrit. "+
			"Note that a NotFound error may indicate authorization issues.\n%v", err)
		return 13
	}

	// And finally, run the test plan generator itself!
	testPlan, err := generator.CreateTestPlan(testReqsConfig, sourceTreeConfig, bbBuilds, changeRevs, repoToSrcRoot)
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

func GetApplication(authOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name: "test_planner",

		Context: func(ctx context.Context) context.Context {
			return ctx
		},

		Commands: []*subcommands.Command{
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),
			cmdGenTestPlan(authOpts),
		},
	}
}

func main() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{gerrit.OAuthScope, auth.OAuthScopeEmail}
	app := GetApplication(opts)
	os.Exit(subcommands.Run(app, nil))
}
