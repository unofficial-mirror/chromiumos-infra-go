// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"context"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"log"
	"os"

	"github.com/maruel/subcommands"
)

var (
	Context = context.Background()
)

func getApplication(authOpts auth.Options) *subcommands.DefaultApplication {
	return &subcommands.DefaultApplication{
		Name:  "branch_util",
		Title: "cros branch tool",
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			getCmdCreateBranch(authOpts),
			getCmdRenameBranch(authOpts),
			getCmdDeleteBranch(authOpts),
			getCmdCreateBranchV2(authOpts),
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

func main() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{gerrit.OAuthScope, auth.OAuthScopeEmail}
	s := &branchApplication{
		getApplication(opts),
		log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)}
	os.Exit(subcommands.Run(s, nil))
}
