#!/usr/bin/env bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "${0%/*}" || (echo "couldn't cd to test directory"; exit 1;)
TEST_DIR=$PWD

source ../common.sh || (echo "couldn't source common.sh"i; exit 1;)

move_test () {
  echo "test: move effect"

  failures=0

  test_bucket=$(create_random_bucket)
  printf "created bucket: %s\n" "$test_bucket"
  move_bucket=$(create_random_bucket)
  printf "created destination bucket: %s\n" "$move_bucket"
  log_bucket=$(create_random_bucket)
  printf "created log bucket: %s\n" "$log_bucket"

  # Make a temp file for the runconfig and update a temp file with the path.
  run_config_tmp=$(mktemp)
  move_bucket_name=$(echo "$move_bucket" | cut -c6-)
  dest_tag=".policy_effect_configuration.move.destination_bucket"
  jq "$dest_tag = \"$move_bucket_name\"" \
    "$TEST_DIR/run_config.json" > "$run_config_tmp"

  log_url="$log_bucket/logs"
  logs_out=$(mktemp -d)

  printf "making objects in bucket\n"
  expected_file_count=10
  expected_rootsizebytes=2048000
  create_random_object_in_bucket "$expected_file_count" 3 "$test_bucket" 400

  json_out=$(mktemp)
  stdout_out=$(mktemp)
  printf "stdout will be in %s and json will be in %s\n" "$stdout_out" \
    "$json_out"

  printf "running cycler\n"
  ./cycler  -bucket "$test_bucket" -iUnderstandCyclerIsInEarlyDevelopment \
                --runConfigPath "$run_config_tmp" \
                --jsonOutFile "$json_out" \
                --runlogURL "$log_url" >"$stdout_out" \
                --mutationAllowed \
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

  # Don't trust cycler itself, use gsutil, this is expected to fail with an
  # expection (somewhat unexpectedly).
  gsutil ls "$test_bucket/**"
  if [[ "$?" -ne "1" ]]; then
    printf "The bucket still had objects in it, should have been moved.\n"
    (( failures++ ))
  fi

  moved_objects=$(gsutil ls "$move_bucket/**" | wc -l)
  if [[ "$moved_objects" -ne "$expected_file_count" ]]; then
    printf "The destination expected %s objects, had %s.\n" \
      "$expected_file_count" "$moved_objects"
    (( failures++ ))
  fi

  printf "emptying destination bucket.\n"
  empty_bucket "$move_bucket"
  printf "removing destination bucket.\n"
  remove_bucket "$move_bucket"

  # clean up
  clean_up_test "$test_bucket" "$log_bucket" "$logs_out" "$stdout_out" \
    "$json_out"

  if [[ $failures -gt 0 ]]; then
    echo "!!had $failures failures!!"
    return $failures
  else
    return 0
  fi
}

move_test
