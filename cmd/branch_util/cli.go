// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"context"
	"log"
	"os"

	"github.com/maruel/subcommands"
)

var (
	Context = context.Background()
)

var application = &subcommands.DefaultApplication{
	Name:  "cros branch",
	Title: "cros branch tool",
	Commands: []*subcommands.Command{
		subcommands.CmdHelp,
		cmdCreateBranch,
		cmdRenameBranch,
		cmdDeleteBranch,
	},
}

type branchApplication struct {
	*subcommands.DefaultApplication
	stdoutLog *log.Logger
	stderrLog *log.Logger
}

func main() {
	s := &branchApplication{
		application,
		log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)}
	os.Exit(subcommands.Run(s, nil))
}
