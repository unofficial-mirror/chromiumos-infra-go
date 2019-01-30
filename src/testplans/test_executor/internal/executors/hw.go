// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package executors

import (
	"context"
	"log"
	"strings"
	"testplans/common/exec"
	model "testplans/common/protos"
)

// ExecuteHW executes hw test plan group in skylab
func ExecuteHW(ctx context.Context, tp *model.TestPlan, dryrun bool) error {
	cmd := make([]string, 0, 0)

	// TODO(yshaul): Replace with reference design when available
	cmd = append(cmd, "-board", tp.SchedulingRequirements.ReferenceDesign)

	cmd = append(cmd, "-pool", "paladin")
	cmd = append(cmd, "-image", tp.BuildPayload.Image[0].ImageName)
	cmd = append(cmd, tp.TestSuite)

	if dryrun {
		log.Printf("skylab %s\n", strings.Join(cmd, " "))
		return nil
	}

	return exec.CommandContext(ctx, "skylab", cmd...).Run()
}
