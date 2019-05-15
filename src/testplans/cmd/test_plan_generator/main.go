// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
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
	"os"
	"strings"
	"testplans/internal/generator"
	"testplans/internal/git"
	"testplans/internal/repo"
)

const (
	sourceTreeTestConfigPath   = "testingconfig/generated/source_tree_test_config.cfg"
	targetTestRequirementsPath = "testingconfig/generated/target_test_requirements.cfg"
)

var (
	unmarshaler = jsonpb.Unmarshaler{AllowUnknownFields: true}
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

func (c *getTestPlanRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	flag.Parse()

	req, err := c.readInputJson()
	if err != nil {
		log.Print(err)
		return 1
	}

	sourceTreeConfig, testReqsConfig, err := c.fetchConfigFromGitiles()
	if err != nil {
		log.Print(err)
		return 2
	}

	bbBuilds, err := readBuildbucketBuilds(req.BuildbucketProtos)
	if err != nil {
		log.Print(err)
		return 3
	}

	changeRevs, err := c.fetchGerritData(bbBuilds)
	if err != nil {
		log.Print(err)
		return 4
	}
	repoToSrcRoot, err := getRepoToSourceRoot(req.ChromiumosCheckoutRoot, req.RepoToolPath)
	if err != nil {
		log.Print(err)
		return 5
	}

	testPlan, err := generator.CreateTestPlan(testReqsConfig, sourceTreeConfig, bbBuilds, changeRevs, *repoToSrcRoot)
	if err != nil {
		log.Printf("Error creating test plan:\n%v", err)
		return 6
	}

	if err = c.writeOutputJson(testPlan); err != nil {
		log.Print(err)
		return 7
	}
	return 0
}

type getTestPlanRun struct {
	subcommands.CommandRunBase
	authFlags  authcli.Flags
	inputJson  string
	outputJson string
}

func (c *getTestPlanRun) readInputJson() (*testplans.GenerateTestPlanRequest, error) {
	inputBytes, err := ioutil.ReadFile(c.inputJson)
	log.Printf("Request is:\n%s", string(inputBytes))
	if err != nil {
		return nil, fmt.Errorf("Failed reading input_json\n%v", err)
	}
	req := &testplans.GenerateTestPlanRequest{}
	if err := unmarshaler.Unmarshal(bytes.NewReader(inputBytes), req); err != nil {
		return nil, fmt.Errorf("Couldn't decode %s as a GenerateTestPlanRequest\n%v", c.inputJson, err)
	}
	return req, nil
}

func (c *getTestPlanRun) fetchConfigFromGitiles() (*testplans.SourceTreeTestCfg, *testplans.TargetTestRequirementsCfg, error) {
	// Create an authenticated client for Gerrit RPCs, then fetch all required CL data from Gerrit.
	ctx := context.Background()
	authOpts, err := c.authFlags.Options()
	if err != nil {
		return nil, nil, err
	}
	authedClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, nil, err
	}
	m, err := git.FetchFilesFromGitiles(authedClient, ctx,
		"chrome-internal.googlesource.com",
		"chromeos/infra/config",
		"master",
		[]string{sourceTreeTestConfigPath, targetTestRequirementsPath})
	if err != nil {
		return nil, nil, err
	}
	sourceTreeConfig := &testplans.SourceTreeTestCfg{}
	if err := unmarshaler.Unmarshal(strings.NewReader((*m)[sourceTreeTestConfigPath]), sourceTreeConfig); err != nil {
		return nil, nil, fmt.Errorf("Couldn't decode %s as a SourceTreeTestCfg\n%v", (*m)[sourceTreeTestConfigPath], err)
	}
	testReqsConfig := &testplans.TargetTestRequirementsCfg{}
	if err := unmarshaler.Unmarshal(strings.NewReader((*m)[targetTestRequirementsPath]), testReqsConfig); err != nil {
		return nil, nil, fmt.Errorf("Couldn't decode %s as a TargetTestRequirementsCfg\n%s",
			targetTestRequirementsPath, err)
	}
	log.Printf("Fetched config from Gitiles: %s\n\n%s",
		proto.MarshalTextString(sourceTreeConfig), proto.MarshalTextString(testReqsConfig))
	return sourceTreeConfig, testReqsConfig, nil
}

func readBuildbucketBuilds(bbBuildsBytes []*testplans.ProtoBytes) ([]*bbproto.Build, error) {
	bbBuilds := make([]*bbproto.Build, 0)
	for _, bbBuildBytes := range bbBuildsBytes {
		bbBuild := &bbproto.Build{}
		if err := proto.Unmarshal(bbBuildBytes.SerializedProto, bbBuild); err != nil {
			return bbBuilds, fmt.Errorf("Couldn't decode %s as a Buildbucket Build\n%v", bbBuildBytes.String(), err)
		}
		bbBuilds = append(bbBuilds, bbBuild)
	}
	if len(bbBuilds) > 0 {
		log.Printf("Sample buildbucket proto:\n%s", proto.MarshalTextString(bbBuilds[0]))
	}
	return bbBuilds, nil
}

func (c *getTestPlanRun) fetchGerritData(bbBuilds []*bbproto.Build) (*git.ChangeRevData, error) {
	// Create an authenticated client for Gerrit RPCs, then fetch all required CL data from Gerrit.
	ctx := context.Background()
	authOpts, err := c.authFlags.Options()
	if err != nil {
		return nil, err
	}
	authedClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}
	changeIds := make([]git.ChangeRevKey, 0)
	for _, build := range bbBuilds {
		for _, ch := range build.Input.GerritChanges {
			changeIds = append(changeIds, git.ChangeRevKey{Host: ch.Host, ChangeNum: ch.Change, Revision: int32(ch.Patchset)})
		}
	}
	chRevData, err := git.GetChangeRevData(authedClient, ctx, changeIds)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch CL data from Gerrit. "+
			"Note that a NotFound error may indicate authorization issues.\n%v", err)
	}
	return chRevData, nil
}

func getRepoToSourceRoot(chromiumosCheckoutRoot, repoToolPath string) (*map[string]string, error) {
	// Run the repo tool to get a mapping from Gerrit project to source root.
	if chromiumosCheckoutRoot == "" {
		log.Printf("Must set request ChromiumosCheckoutRoot")
		return nil, errors.New("Must set request ChromiumosCheckoutRoot")
	}
	// If the path isn't set, assume repo is just on the caller's $PATH.
	if repoToolPath == "" {
		repoToolPath = "repo"
	}
	repoToSrcRoot, err := repo.GetRepoToSourceRoot(chromiumosCheckoutRoot, repoToolPath)
	if err != nil {
		return nil, fmt.Errorf("Error with repo tool call\n%v", err)
	}
	return &repoToSrcRoot, nil
}

func (c *getTestPlanRun) writeOutputJson(tp *testplans.GenerateTestPlanResponse) error {
	marshal := &jsonpb.Marshaler{EmitDefaults: true, Indent: "  "}
	jsonOutput, err := marshal.MarshalToString(tp)
	if err != nil {
		return fmt.Errorf("Failed to marshal %v\n%v", tp, err)
	}
	if err = ioutil.WriteFile(c.outputJson, []byte(jsonOutput), 0644); err != nil {
		return fmt.Errorf("Failed to write output JSON!\n%v", err)
	}
	log.Printf("Full test plan =\n%s", proto.MarshalTextString(tp))
	log.Printf("Wrote output to %s", c.outputJson)
	return nil
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
