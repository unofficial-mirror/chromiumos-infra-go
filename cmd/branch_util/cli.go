package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/maruel/subcommands"
)

var (
	Context      = context.Background()
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
	log *log.Logger
}

func main() {
	flag.StringVar(&RepoToolPath, "repo_path", "repo", "Path of repo tool. Optional if repo is on your PATH.")
	s := &branchApplication{application, log.New(ioutil.Discard, "", log.LstdFlags|log.Lmicroseconds)}
	os.Exit(subcommands.Run(s, nil))
}
