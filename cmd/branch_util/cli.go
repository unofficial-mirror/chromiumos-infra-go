// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"context"
	"fmt"
	"github.com/maruel/subcommands"
	"go.chromium.org/chromiumos/infra/go/internal/branch"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"log"
	"os"
)

var (
	Context            = context.Background()
	workerCount        int
	defaultManifestUrl = "https://chrome-internal.googlesource.com/chromeos/manifest-internal"
)

func getApplication(authOpts auth.Options) *subcommands.DefaultApplication {
	return &subcommands.DefaultApplication{
		Name:  "branch_util",
		Title: "cros branch tool",
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			getCmdRenameBranch(authOpts),
			getCmdDeleteBranch(authOpts),
			getCmdCreateBranch(authOpts),
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),
		},
	}
}

type branchApplication struct {
	*subcommands.DefaultApplication
	stdoutLog *log.Logger
	stderrLog *log.Logger
}

type branchCommand interface {
	validate([]string) (bool, string)
	getRoot() string
	getManifestUrl() string
}

// Common flags
type CommonFlags struct {
	subcommands.CommandRunBase
	Push        bool
	Force       bool
	Root        string
	ManifestUrl string
	authFlags   authcli.Flags
}

func (c *CommonFlags) InitFlags(authOpts auth.Options) {
	// Common flags
	c.Flags.BoolVar(&c.Push, "push", false,
		"Push branch modifications to remote repos. Before setting this flag, "+
			"ensure that you have the proper permissions and that you know what "+
			"you are doing. Ye be warned.")
	c.Flags.BoolVar(&c.Force, "force", false,
		"Required for any remote operation that would delete an existing "+
			"branch. Also required when trying to branch from a previously "+
			"branched manifest version.")
	// Sync CheckoutOptions
	c.Flags.StringVar(&c.ManifestUrl, "manifest-url", defaultManifestUrl,
		"URL of the manifest to be checked out. Defaults to googlesource URL "+
			"for manifest-internal.")
	c.Flags.IntVar(&workerCount, "j", 1, "Number of jobs to run for parallel operations.")
	c.authFlags.Register(c.GetFlags(), authOpts)
}

func Run(c branchCommand, a subcommands.Application, args []string, env subcommands.Env) int {
	branch.StdoutLog = a.(*branchApplication).stdoutLog
	branch.StderrLog = a.(*branchApplication).stderrLog
	// Set output of standard log in case any packages use it.
	if branch.StdoutLog != nil {
		log.SetOutput(branch.StdoutLog.Writer())
	}

	// Validate flags/arguments.
	ok, errMsg := c.validate(args)
	if !ok {
		fmt.Fprintf(a.GetErr(), errMsg+"\n")
		return 1
	}

	return 0
}

func main() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{gerrit.OAuthScope, auth.OAuthScopeEmail}
	s := &branchApplication{
		getApplication(opts),
		log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)}
	os.Exit(subcommands.Run(s, nil))
}
