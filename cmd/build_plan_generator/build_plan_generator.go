package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/build_plan"
	igerrit "go.chromium.org/chromiumos/infra/go/internal/gerrit"
	"go.chromium.org/chromiumos/infra/go/internal/repo"
	cros_pb "go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const (
	buildIrrelevanceConfigPath = "testingconfig/generated/build_irrelevance_config.cfg"
)

var (
	unmarshaler = jsonpb.Unmarshaler{AllowUnknownFields: true}
)

func cmdCheckSkip(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "generate-plan --input_json=/path/to/input.json --output_json=/path/to/output.json",
		ShortDesc: "Checks which of the provided BuilderConfigs must be run",
		LongDesc:  "Checks which of the provided BuilderConfigs must be run.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkBuild{}
			c.authFlags = authcli.Flags{}
			c.authFlags.Register(c.GetFlags(), authOpts)
			c.Flags.StringVar(&c.inputJson, "input_json", "",
				"Path to JSON proto representing a GenerateBuildPlanRequest")
			c.Flags.StringVar(&c.outputJson, "output_json", "",
				"Path to file to write output GenerateBuildPlanResponse JSON proto")
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

	cfg, err := c.fetchConfigFromGitiles()
	if err != nil {
		log.Print(err)
		return 2
	}

	changes, err := readGerritChanges(req.GerritChanges)
	if err != nil {
		log.Print(err)
		return 3
	}

	changeRevs, err := c.fetchGerritData(changes)
	if err != nil {
		log.Print(err)
		return 4
	}
	repoToSrcRoot, err := c.getRepoToSourceRoot(req.ManifestCommit)
	if err != nil {
		log.Print(err)
		return 5
	}

	resp, err := build_plan.CheckBuilders(req.BuilderConfigs, changes, changeRevs, *repoToSrcRoot, *cfg)
	if err != nil {
		log.Printf("Error checking which builds can be skipped:\n%v", err)
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

func (c *checkBuild) readInputJson() (*cros_pb.GenerateBuildPlanRequest, error) {
	inputBytes, err := ioutil.ReadFile(c.inputJson)
	log.Printf("Request is:\n%s", string(inputBytes))
	if err != nil {
		return nil, fmt.Errorf("Failed reading input_json\n%v", err)
	}
	req := &cros_pb.GenerateBuildPlanRequest{}
	if err := unmarshaler.Unmarshal(bytes.NewReader(inputBytes), req); err != nil {
		return nil, fmt.Errorf("Couldn't decode %s as a chromiumos.GenerateBuildPlanRequest\n%v", c.inputJson, err)
	}
	return req, nil
}

func (c *checkBuild) fetchConfigFromGitiles() (*testplans_pb.BuildIrrelevanceCfg, error) {
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
	m, err := igerrit.FetchFilesFromGitiles(authedClient, ctx,
		"chrome-internal.googlesource.com",
		"chromeos/infra/config",
		"master",
		[]string{buildIrrelevanceConfigPath})
	if err != nil {
		return nil, err
	}
	buildIrrelevanceConfig := &testplans_pb.BuildIrrelevanceCfg{}
	if err := unmarshaler.Unmarshal(strings.NewReader((*m)[buildIrrelevanceConfigPath]), buildIrrelevanceConfig); err != nil {
		return nil, fmt.Errorf("Couldn't decode %s as a BuildIrrelevanceCfg\n%v", (*m)[buildIrrelevanceConfigPath], err)
	}
	log.Printf("Fetched config from Gitiles: %s\n", proto.MarshalTextString(buildIrrelevanceConfig))
	return buildIrrelevanceConfig, nil
}

func readGerritChanges(changeBytes []*cros_pb.ProtoBytes) ([]*bbproto.GerritChange, error) {
	changes := make([]*bbproto.GerritChange, 0)
	for i, c := range changeBytes {
		gc := &bbproto.GerritChange{}
		if err := proto.Unmarshal(c.SerializedProto, gc); err != nil {
			return nil, fmt.Errorf("Couldn't decode %s as a GerritChange\n%v", c.String(), err)
		}
		log.Printf("Got GerritChange %d proto:\n%s", i, proto.MarshalTextString(gc))
		changes = append(changes, gc)
	}
	return changes, nil
}

func (c *checkBuild) fetchGerritData(changes []*bbproto.GerritChange) (*igerrit.ChangeRevData, error) {
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
	for _, ch := range changes {
		changeIds = append(changeIds, igerrit.ChangeRevKey{Host: ch.Host, ChangeNum: ch.Change, Revision: int32(ch.Patchset)})
	}
	chRevData, err := igerrit.GetChangeRevData(authedClient, ctx, changeIds)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch CL data from Gerrit. "+
			"Note that a NotFound error may indicate authorization issues.\n%v", err)
	}
	return chRevData, nil
}

func (c *checkBuild) getRepoToSourceRoot(manifestCommit string) (*map[string]map[string]string, error) {
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
	repoToRemoteBranchToSrcRoot, err := repo.GetRepoToRemoteBranchToSourceRootFromManifests(authedClient, ctx, manifestCommit)
	if err != nil {
		return nil, fmt.Errorf("Error with repo tool call\n%v", err)
	}
	return &repoToRemoteBranchToSrcRoot, nil
}

func (c *checkBuild) writeOutputJson(resp *cros_pb.GenerateBuildPlanResponse) error {
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
		Name: "build_plan_generator",

		Context: func(ctx context.Context) context.Context {
			return ctx
		},

		Commands: []*subcommands.Command{
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),
			cmdCheckSkip(authOpts),
		},
	}
}

func main() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{gerrit.OAuthScope, auth.OAuthScopeEmail}
	app := GetApplication(opts)
	os.Exit(subcommands.Run(app, nil))
}
