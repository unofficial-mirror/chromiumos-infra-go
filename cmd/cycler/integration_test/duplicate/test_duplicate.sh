#!/usr/bin/env bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "${0%/*}" || (echo "couldn't cd to test directory"; exit 1;)
TEST_DIR=$PWD

source ../common.sh || (echo "couldn't source common.sh"i; exit 1;)

dup_test () {
  echo "test: duplicate effect"

  failures=0

  test_bucket=$(create_random_bucket)
  printf "created bucket: %s\n" "$test_bucket"
  dup_bucket=$(create_random_bucket)
  printf "created destination bucket: %s\n" "$dup_bucket"
  log_bucket=$(create_random_bucket)
  printf "created log bucket: %s\n" "$log_bucket"

  # Make a temp file for the runconfig and update a temp file with the path.
  run_config_tmp=$(mktemp)
  dup_bucket_name=$(echo "$dup_bucket" | cut -c6-)
  dest_tag=".policy_effect_configuration.duplicate.destination_bucket"
  jq "$dest_tag = \"$dup_bucket_name\"" \
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

  orig_objects=$(gsutil ls "$dup_bucket/**" | wc -l)
  if [[ "$orig_objects" -ne "$expected_file_count" ]]; then
    printf "The source expected %s objects, had %s.\n" \
      "$expected_file_count" "$orig_objects"
    (( failures++ ))
  fi

  duped_objects=$(gsutil ls "$dup_bucket/**" | wc -l)
  if [[ "$duped_objects" -ne "$expected_file_count" ]]; then
    printf "The destination expected %s objects, had %s.\n" \
      "$expected_file_count" "$duped_objects"
    (( failures++ ))
  fi

  printf "emptying destination bucket.\n"
  empty_bucket "$dup_bucket"
  printf "removing destination bucket.\n"
  remove_bucket "$dup_bucket"

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

dup_test
