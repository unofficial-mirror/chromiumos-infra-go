// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package executors

import (
	"context"
	"fmt"
	model "testplans/common/protos"
)

type execFn func(ctx context.Context, tp *model.TestPlan, dryrun bool) error

var (
	executors = make(map[string]execFn)
)

func init() {
	executors["hw"] = ExecuteHW
}

// Execute executes supplied test plan
func Execute(ctx context.Context, combined *model.CombinedTestPlan, dryrun bool) error {
	for _, tp := range combined.TestPlan {
		fn, ok := executors[tp.TestEnv]
		if !ok {
			return fmt.Errorf("Unknown test env '%s'", tp.TestEnv)
		}

		if err := fn(ctx, tp, dryrun); err != nil {
			return err
		}
	}

	return nil
}
