// Copyright 2018 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
// Command cros-admin is the Chrome OS infrastructure admin tool.
package main

import (
	"context"
	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
	"os"
)

func getApplication() *cli.Application {
	return &cli.Application{
		Name:  "test_executor",
		Title: "Chrome OS Infra Test Plan Executor",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
		},
	}
}

func main() {
	os.Exit(subcommands.Run(getApplication(), nil))
}
