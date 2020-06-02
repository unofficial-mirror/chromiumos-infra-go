#!/usr/bin/env bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "${0%/*}" || (echo "couldn't cd to test directory"; exit 1;)
TEST_DIR=$PWD

source ../common.sh || (echo "couldn't source common.sh"i; exit 1;)

expected_size_test() {
  echo "test: noop policy and basic cycler functionality"

  failures=0

  test_bucket=$(create_random_bucket)
  log_bucket=$(create_random_bucket)
  log_url="$log_bucket/logs"
  printf "created bucket: %s\n" "$test_bucket"

  printf "making objects in bucket\n"
  create_random_object_in_bucket 10 3 "$test_bucket" 400

  json_out=$(mktemp)
  stdout_out=$(mktemp)

  printf "running cycler\n"
  ./cycler  -bucket "$test_bucket" -iUnderstandCyclerIsInEarlyDevelopment \
                --runConfigPath "$TEST_DIR/run_config.json" \
                --jsonOutFile "$json_out" \
                --runlogURL "$log_url" >"$stdout_out" \
                || die 1 "cycler run exited non-zero"

  if [[ "$VERBOSE" = true ]]; then
    echo "cat $json_out" | jq
  fi

  run_uuid=$(jq -r '.RunUUID' "$json_out")
  printf "run uuid: %s\n" "$run_uuid"

  # make some assertions on the output gathered.
  expected_rootsizebytes=2048000
  actual_rootsizebytes=$(jq '.PrefixStats.RootSizeBytes' "$json_out")
  if [[ "$expected_rootsizebytes" -ne "$actual_rootsizebytes" ]]; then
    printf ".PrefixStats.RootSizeByte %s, " "$actual_rootsizebytes\n"
    printf "expected %s " "$expected_rootsizebytes\n"
    (( failures++ ))
  else
    echo "matched expected size"
  fi

  expected_file_count=10
  actual_file_count=$(jq '.ActionStats.SizeBytesHistogram.Count' "$json_out")
  if [[ "$expected_file_count" -ne "$actual_file_count" ]]; then
    printf ".SizeBytesHistogram.Count %s" "$actual_file_count\n"
    printf "expected %s" "$expected_file_count\n"
    (( failures++ ))
  else
    echo "matched histogram count size"
  fi

  # inspect the logging directory, ensure files logged.
  logs_out=$(mktemp -d)
  printf "copying logs locally\n"
  gsutil -m cp -R "$log_bucket/logs/$run_uuid" "$logs_out" >/dev/null 2>&1

  # TODO(engeg@): clean up tmp dir, inspect bucket objects.

  # clean up
  clean_up_test "$test_bucket" "$log_bucket"

  if [[ failures -gt 0 ]]; then
    return 1
  else
    return 0
  fi
}

expected_size_test
