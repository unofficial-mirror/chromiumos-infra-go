// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package exec

import (
	"context"
	"os"
	"os/exec"
)

// CommandMock captures the args passed to CommandContext
type CommandMock struct {
	name       string
	args       []string
	cmdContext commandContextFn
}

// Args returns command args
func (c *CommandMock) Args() []string {
	return c.args
}

// Name returns the name of the command called
func (c *CommandMock) Name() string {
	return c.name
}

// Close restores the internal CommandContext
func (c *CommandMock) Close() {
	commandContext = c.cmdContext
}

// used to "mock" subprocesses without polluting callers... *sigh*
func (c *CommandMock) commandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	c.name = name
	c.args = args

	cs := []string{"-test.run=DummyTest"}
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	return cmd
}

// MockCommandContext mocks the internal CommandContext
func MockCommandContext() *CommandMock {
	newCmdCtx := &CommandMock{
		cmdContext: commandContext,
	}

	commandContext = newCmdCtx.commandContext
	return newCmdCtx
}
