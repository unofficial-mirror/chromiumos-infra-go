#!/bin/bash
# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Runs all of the Go tests in this module and verifies all binaries compile.
# This file shouldn't really be necessary. We should be able to just run
# "go test .", but the module definition isn't quite right at the moment.

# Move to this script's directory.
cd "$(dirname "$0")"

echo "Running tests"
if ! go test ./...; then
  exit 1
fi

echo "Checking that binaries compile"
if ! go build ./...; then
  exit 1
fi

echo "Vetting the code"
if ! go vet ./...; then
  exit 1
fi

echo "Done"
