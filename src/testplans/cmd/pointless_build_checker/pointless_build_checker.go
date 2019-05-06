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
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"io/ioutil"
	"log"
	"os"
	"testplans/internal/git"
	"testplans/internal/pointless"
	"testplans/internal/repo"
)

var (
	// TODO(crbug.com/956577): Define a config schema and move this list into Starlark, where it can
	// be expanded upon.
	buildIrrelevantPaths = []string{"chromite/config"}
)

func cmdCheckBuild(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "check-build --input_json=/path/to/input.json --output_json=/path/to/output.json",
		ShortDesc: "Checks if the current build is pointless",
		LongDesc: "Checks if the current build is pointless, e.g. if the commits in the CQ run can't " +
			"actually affect the outcome of the build.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkBuild{}
			c.authFlags = authcli.Flags{}
			c.authFlags.Register(c.GetFlags(), authOpts)
			c.Flags.StringVar(&c.inputJson, "input_json", "",
				"Path to JSON proto representing a PointlessBuildCheckRequest")
			c.Flags.StringVar(&c.outputJson, "output_json", "",
				"Path to file to write output PointlessBuildCheckResponse JSON proto")
			return c
		}}
}

func (c *checkBuild) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	flag.Parse()

	req, err := c.readInputJson()
	if err != nil {
		log.Print(err)
		return 1
	}

	build, err := readBuildbucketBuild(req.BuildbucketProto)
	if err != nil {
		log.Print(err)
		return 3
	}

	changeRevs, err := c.fetchGerritData(build)
	if err != nil {
		log.Print(err)
		return 4
	}
	repoToSrcRoot, err := getRepoToSourceRoot(req.ChromiumosWorkspaceCheckoutRoot, req.RepoToolPath)
	if err != nil {
		log.Print(err)
		return 5
	}

	resp, err := pointless.CheckBuilder(build, changeRevs, req.DepGraph, *repoToSrcRoot, buildIrrelevantPaths)
	if err != nil {
		log.Printf("Error checking if build is pointless:\n%v", err)
		return 6
	}

	if err = c.writeOutputJson(resp); err != nil {
		log.Print(err)
		return 7
	}
	return 0
}

type checkBuild struct {
	subcommands.CommandRunBase
	authFlags  authcli.Flags
	inputJson  string
	outputJson string
}

func (c *checkBuild) readInputJson() (*testplans_pb.PointlessBuildCheckRequest, error) {
	inputBytes, err := ioutil.ReadFile(c.inputJson)
	log.Printf("Request is:\n%s", string(inputBytes))
	if err != nil {
		return nil, fmt.Errorf("Failed reading input_json\n%v", err)
	}
	req := &testplans_pb.PointlessBuildCheckRequest{}
	if err := jsonpb.Unmarshal(bytes.NewReader(inputBytes), req); err != nil {
		return nil, fmt.Errorf("Couldn't decode %s as a chromiumos.PointlessBuildCheckRequest\n%v", c.inputJson, err)
	}
	return req, nil
}

func readBuildbucketBuild(bbBuildBytes *testplans_pb.ProtoBytes) (*bbproto.Build, error) {
	bbBuild := &bbproto.Build{}
	if err := proto.Unmarshal(bbBuildBytes.SerializedProto, bbBuild); err != nil {
		return nil, fmt.Errorf("Couldn't decode %s as a Buildbucket Build\n%v", bbBuildBytes.String(), err)
	}
	log.Printf("Got buildbucket proto:\n%s", proto.MarshalTextString(bbBuild))
	return bbBuild, nil
}

func (c *checkBuild) fetchGerritData(build *bbproto.Build) (*git.ChangeRevData, error) {
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
	for _, ch := range build.Input.GerritChanges {
		changeIds = append(changeIds, git.ChangeRevKey{Host: ch.Host, ChangeNum: ch.Change, Revision: int32(ch.Patchset)})
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

func (c *checkBuild) writeOutputJson(resp *testplans_pb.PointlessBuildCheckResponse) error {
	marshal := &jsonpb.Marshaler{EmitDefaults: true, Indent: "  "}
	jsonOutput, err := marshal.MarshalToString(resp)
	if err != nil {
		return fmt.Errorf("Failed to marshal %v\n%v", resp, err)
	}
	if err = ioutil.WriteFile(c.outputJson, []byte(jsonOutput), 0644); err != nil {
		return fmt.Errorf("Failed to write output JSON!\n%v", err)
	}
	log.Printf("Full output =\n%s", proto.MarshalTextString(resp))
	log.Printf("Wrote output to %s", c.outputJson)
	return nil
}

func GetApplication(authOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name: "pointless_build_checker",

		Context: func(ctx context.Context) context.Context {
			return ctx
		},

		Commands: []*subcommands.Command{
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),
			cmdCheckBuild(authOpts),
		},
	}
}

func main() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{gerrit.OAuthScope, auth.OAuthScopeEmail}
	app := GetApplication(opts)
	os.Exit(subcommands.Run(app, nil))
}
