#!/usr/bin/env bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "${0%/*}" || (echo "couldn't cd to test directory"; exit 1;)
TEST_DIR=$PWD

source ../common.sh || (echo "couldn't source common.sh"i; exit 1;)

noop_test () {
  echo "test: noop policy and basic cycler functionality"

  failures=0

  test_bucket=$(create_random_bucket)
  log_bucket=$(create_random_bucket)
  log_url="$log_bucket/logs"
  logs_out=$(mktemp -d)

  printf "created bucket: %s\n" "$test_bucket"

  printf "making objects in bucket\n"
  expected_file_count=10
  expected_rootsizebytes=2048000
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

  # inspect the logging directory, ensure files logged.
  printf "copying logs locally to: %s\n" "$logs_out"
  gsutil -m cp -R "$log_bucket/logs/$run_uuid" "$logs_out" >/dev/null 2>&1

  common_checks "$expected_file_count" "$expected_rootsizebytes" "$logs_out" \
    "$json_out"
  (( failures += $? ))

  # clean up
  clean_up_test "$test_bucket" "$log_bucket" "$logs_out" "$stdout_out" \
    "$json_out"

  if [[ $failures -gt 0 ]]; then
    echo "had $failures failures"
    return $failures
  else
    return 0
  fi
}

noop_test
