// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package test_util

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"

	"go.chromium.org/chromiumos/infra/go/internal/cmd"
	"go.chromium.org/luci/common/errors"
)

var (
	CommandRunnerImpl cmd.CommandRunner = cmd.RealCommandRunner{}
)

// UnorderedEqual checks that the two arrays contain the same elements, but
// they don't have to be the same order.
func UnorderedEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]int)
	for _, v := range a {
		am[v]++
	}
	bm := make(map[string]int)
	for _, v := range b {
		bm[v]++
	}
	return reflect.DeepEqual(am, bm)
}

// UnorderedContains checks that arr has certain elements.
func UnorderedContains(arr, has []string) bool {
	elts := make(map[string]int)
	for _, v := range arr {
		elts[v]++
	}
	for _, elt := range has {
		_, ok := elts[elt]
		if !ok {
			return false
		}
	}
	return true
}

// AssertContentsEqual checks that there's no difference between two directories/files.
func AssertContentsEqual(path_a, path_b string) error {
	ai, err := os.Stat(path_a)
	if err != nil {
		return errors.Annotate(err, "failed to stat %s", path_a).Err()
	}
	bi, err := os.Stat(path_b)
	if err != nil {
		return errors.Annotate(err, "failed to stat %s", path_b).Err()
	}
	// diff -rq works for dirs and files, but only if both operands are the same.
	if ai.IsDir() && !bi.IsDir() {
		return fmt.Errorf("%s is a dir and %s is a file", path_a, path_b)
	}
	if !ai.IsDir() && bi.IsDir() {
		return fmt.Errorf("%s is a file and %s is a dir", path_a, path_b)
	}

	ctx := context.Background()
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := []string{"-rq", path_a, path_b}
	if err = CommandRunnerImpl.RunCommand(ctx, &stdoutBuf, &stderrBuf, "", "diff", cmd...); err != nil {
		if err.Error() == "exit status 1" {
			return fmt.Errorf("files differ")
		} else {
			return fmt.Errorf("error running diff")
		}
	}
	return err
}
