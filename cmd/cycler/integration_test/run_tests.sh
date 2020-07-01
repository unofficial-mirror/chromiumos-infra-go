#!/bin/bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Executes all the integration tests, assumed to be at least one directory deep
# and have the common prefix of test_*.sh. It will sum up the return codes of
# each one and return that (with a return code of 0 ultimately determining
# integration test success).

cd "${0%/*}" || (echo "couldn't cd to script directory"; exit 1;)

failure_sum=0
tests=$(find . -type f -name 'test_*.sh' -exec echo {} \;)

printf "== Found the following tests ==\n%s\n" "$tests"

while IFS= read -r line; do
  echo "== Running the test $line =="
  $line
  (( failure_sum += $? ))
done <<< "$tests"

echo "== Summary =="
echo "There were $failure_sum failures during integration test execution."
exit $failure_sum
