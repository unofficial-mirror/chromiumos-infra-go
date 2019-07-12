// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
)

type CommandRunner interface {
	RunCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error
}

type RealCommandRunner struct{}

func (c RealCommandRunner) RunCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	cmd.Dir = dir
	return cmd.Run()
}

type FakeCommandRunner struct {
	Stdout      string
	Stderr      string
	ExpectedCmd []string
	ExpectedDir string
	FailCommand bool
}

func (c FakeCommandRunner) RunCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error {
	stdoutBuf.WriteString(c.Stdout)
	stderrBuf.WriteString(c.Stderr)
	cmd := append([]string{name}, args...)
	if len(c.ExpectedCmd) > 0 {
		if !reflect.DeepEqual(cmd, c.ExpectedCmd) {
			expectedCmd := strings.Join(c.ExpectedCmd, " ")
			actualCmd := strings.Join(cmd, " ")
			return fmt.Errorf("wrong cmd; expected %s got %s", expectedCmd, actualCmd)
		}
	}
	if c.ExpectedDir != "" {
		if dir != c.ExpectedDir {
			return fmt.Errorf("wrong cmd dir; expected %s got %s", c.ExpectedDir, dir)
		}
	}
	if c.FailCommand {
		return &exec.ExitError{}
	}
	return nil
}

type FakeCommandRunnerMulti struct {
	run            int
	CommandRunners []FakeCommandRunner
}

func (c *FakeCommandRunnerMulti) RunCommand(ctx context.Context, stdoutBuf, stderrBuf *bytes.Buffer, dir, name string, args ...string) error {
	if c.run >= len(c.CommandRunners) {
		return fmt.Errorf("unexpected cmd.")
	}
	err := c.CommandRunners[c.run].RunCommand(ctx, stdoutBuf, stderrBuf, dir, name, args...)
	c.run += 1
	return err
}
