// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package exec

import (
	"context"
	"os/exec"
)

type commandContextFn func(ctx context.Context, name string, args ...string) *exec.Cmd

// commandContext wraps exec.CommandContext; it's overridden in tests.
var commandContext commandContextFn

// CommandContext returns exec.CommandContext at runtime; it returns a mock under test.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return commandContext(ctx, name, args...)
}

func init() {
	commandContext = exec.CommandContext
}
