// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/generator"
	igerrit "go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
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
			c.Flags.StringVar(&c.inputBinaryPb, "input_binary_pb", "", "Path to binaryproto file representing a GenerateTestPlanRequest")
			c.Flags.StringVar(&c.outputBinaryPb, "output_binary_pb", "", "Path to file to write output GenerateTestPlanResponse binaryproto")
			return c
		}}
}

func (c *getTestPlanRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	flag.Parse()

	req, err := c.readInput()
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

	gerritChanges, err := readGerritChanges(req.GerritChanges)
	if err != nil {
		log.Print(err)
		return 8
	}

	changeRevs, err := c.fetchGerritData(gerritChanges)
	if err != nil {
		log.Print(err)
		return 4
	}
	repoToSrcRoot, err := c.getRepoToSourceRoot(req.ManifestCommit)
	if err != nil {
		log.Print(err)
		return 5
	}

	testPlan, err := generator.CreateTestPlan(testReqsConfig, sourceTreeConfig, bbBuilds, gerritChanges, changeRevs, *repoToSrcRoot)
	if err != nil {
		log.Printf("Error creating test plan:\n%v", err)
		return 6
	}

	if err = c.writeOutput(testPlan); err != nil {
		log.Print(err)
		return 7
	}
	return 0
}

type getTestPlanRun struct {
	subcommands.CommandRunBase
	authFlags      authcli.Flags
	inputJson      string
	outputJson     string
	inputBinaryPb  string
	outputBinaryPb string
}

func (c *getTestPlanRun) readInput() (*testplans.GenerateTestPlanRequest, error) {
	// use input_binary_pb if it's specified
	if len(c.inputBinaryPb) > 0 {
		inputPb, err := ioutil.ReadFile(c.inputBinaryPb)
		if err != nil {
			return nil, fmt.Errorf("Failed reason input_binary_pb\n%v", err)
		}
		req := &testplans.GenerateTestPlanRequest{}
		if err := proto.Unmarshal(inputPb, req); err != nil {
			return nil, fmt.Errorf("Failed parsing input_binary_pb as proto\n%v", err)
		}
		return req, nil
		// otherwise use input_json
	} else {
		inputBytes, err := ioutil.ReadFile(c.inputJson)
		if err != nil {
			return nil, fmt.Errorf("Failed reading input_json\n%v", err)
		}
		req := &testplans.GenerateTestPlanRequest{}
		if err := unmarshaler.Unmarshal(bytes.NewReader(inputBytes), req); err != nil {
			return nil, fmt.Errorf("Couldn't decode %s as a GenerateTestPlanRequest\n%v", c.inputJson, err)
		}
		return req, nil
	}
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
	m, err := igerrit.FetchFilesFromGitiles(authedClient, ctx,
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

func readGerritChanges(changesBytes []*testplans.ProtoBytes) ([]*bbproto.GerritChange, error) {
	changes := make([]*bbproto.GerritChange, 0)
	for _, changeBytes := range changesBytes {
		change := &bbproto.GerritChange{}
		if err := proto.Unmarshal(changeBytes.SerializedProto, change); err != nil {
			return changes, fmt.Errorf("Couldn't decode %s as a GerritChange\n%v", changeBytes.String(), err)
		}
		changes = append(changes, change)
	}
	if len(changes) > 0 {
		log.Printf("Sample GerritChange proto:\n%s", proto.MarshalTextString(changes[0]))
	}
	return changes, nil
}

func (c *getTestPlanRun) fetchGerritData(changes []*bbproto.GerritChange) (*igerrit.ChangeRevData, error) {
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
	changeIds := make([]igerrit.ChangeRevKey, 0)
	for _, c := range changes {
		changeIds = append(changeIds, igerrit.ChangeRevKey{Host: c.Host, ChangeNum: c.Change, Revision: int32(c.Patchset)})
	}
	chRevData, err := igerrit.GetChangeRevData(authedClient, ctx, changeIds)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch CL data from Gerrit. "+
				"Note that a NotFound error may indicate authorization issues.\n%v", err)
	}
	return chRevData, nil
}

func (c *getTestPlanRun) getRepoToSourceRoot(manifestCommit string) (*map[string]map[string]string, error) {
	ctx := context.Background()
	authOpts, err := c.authFlags.Options()
	if err != nil {
		return nil, err
	}
	authedClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}
	if manifestCommit == "" {
		log.Print("No manifestCommit provided. Using 'snapshot' instead.")
		manifestCommit = "snapshot"
	}
	repoToSrcRoot, err := repo.GetRepoToRemoteBranchToSourceRootFromManifests(authedClient, ctx, manifestCommit)
	if err != nil {
		return nil, fmt.Errorf("Error with repo tool call\n%v", err)
	}
	return &repoToSrcRoot, nil
}

func (c *getTestPlanRun) writeOutput(tp *testplans.GenerateTestPlanResponse) error {
	if len(c.outputJson) > 0 {
		marshal := &jsonpb.Marshaler{EmitDefaults: true, Indent: "  "}
		jsonOutput, err := marshal.MarshalToString(tp)
		if err != nil {
			return fmt.Errorf("Failed to marshal JSON %v\n%v", tp, err)
		}
		if err = ioutil.WriteFile(c.outputJson, []byte(jsonOutput), 0644); err != nil {
			return fmt.Errorf("Failed to write output JSON!\n%v", err)
		}
		log.Printf("Wrote output JSON to %s", c.outputJson)
	}

	if len(c.outputBinaryPb) > 0 {
		binaryOutput, err := proto.Marshal(tp)
		if err != nil {
			return fmt.Errorf("Failed to marshal binaryproto %v\n%v", tp, err)
		}
		if err = ioutil.WriteFile(c.outputBinaryPb, binaryOutput, 0644); err != nil {
			return fmt.Errorf("Failed to write output binary proto!\n%v", err)
		}
		log.Printf("Wrote output binary proto to %s", c.outputBinaryPb)
	}

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
