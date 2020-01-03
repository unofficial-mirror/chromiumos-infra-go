#!/usr/bin/env bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

source ../common.sh

expected_size_test() {
  echo "Testing noop policy and basic cycler functionality"
  echo "=================================================="

  failures=0

  test_bucket=$(create_random_bucket)
  log_bucket=$(create_random_bucket)
  log_url="$log_bucket/logs"
  printf "created bucket: %s\n" "$test_bucket"

  printf "making objects in bucket\n"
  create_random_object_in_bucket 10 3 "$test_bucket" 400

  json_out=$(mktemp)

  ../../cycler  -bucket "$test_bucket" -iUnderstandCyclerIsInEarlyDevelopment \
                --runConfigPath ./run_config.json \
                --jsonOutFile "$json_out" --runlogURL "$log_url" >/dev/null 2>&1

  # make some assertions on the output gathered.
  expected_rootsizebytes=2048000
  actual_rootsizebytes=$(jq '.PrefixStats.RootSizeBytes' "$json_out")
  if [[ "$expected_rootsizebytes" -ne "$actual_rootsizebytes" ]]; then
    echo "rootsizebytes $actual_rootsizebytes, expected $expected_rootsizebytes"
    (( failures++ ))
  else
    echo "matched expected size"
  fi

  expected_file_count=10
  actual_file_count=$(jq '.SizeBytesHistogram.Count' "$json_out")
  if [[ "$expected_file_count" -ne "$actual_file_count" ]]; then
    echo "counted $actual_file_count, expected $expected_file_count"
    (( failures++ ))
  else
    echo "matched histogram count size"
  fi

  # TODO(engeg@), we can inspect that logging worked here.

  # clean up
  clean_up_test "$test_bucket" "$log_bucket"

  if [[ failures -gt 0 ]]; then
    return 1
  else
    return 0
  fi
}

expected_size_test