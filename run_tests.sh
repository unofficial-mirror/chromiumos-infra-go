#!/bin/bash
# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Runs all of the Go tests in this module.
# This file shouldn't really be necessary. We should be able to just run
# "go test .", but the module definition isn't quite right at the moment.

# Move to this script's directory.
cd "$(dirname "$0")"

test_dirs=$(find . -name '*_test.go' -exec dirname {} \; | sort | uniq)
for d in $test_dirs; do
  echo "Testing ${d}"
  go test $d/*
done

