#!/usr/bin/env bash

# Copyright 2020 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

warn () {
    echo "$0:" "$@" >&2
}

die () {
    rc=$1
    shift
    warn "$@"
    exit "$rc"
}

USAGE=$(cat <<EOF
Usage: ./test_effect/test_name.sh [-v] [-d]
    -v: Run test in verbose mode
    -x: Set -x and be extremely verbose
    -d: Don't rebuild cycler before running
    -l: Leave downloaded cycler logs
EOF
)

usage_and_die() { echo "$USAGE"; exit 1; }

if [[ $(basename "$0") = "common.sh" ]]; then
 echo "don't invoke common.sh directly"
 usage_and_die
fi

# We only allow tests to nest a single directory.
cd .. || die 1 "couldn't cd to top level test directory"

REBUILD=true
SETX=false
LEAVELOGS=false

# Initializes the options for the test harness script.
while getopts "vdxl" o; do
  case "${o}" in
    v)
      VERBOSE=true
      echo "verbose mode: $VERBOSE"
      ;;
    x)
      SETX=true
      echo "setting -x (extremely verbose)"
      ;;
    l)
      LEAVELOGS=true
      ;;
    d)
      REBUILD=false
      ;;
    *)
      usage_and_die
      ;;
   esac
done

if [[ $SETX = true ]]; then
  set -x
fi

if [[ $REBUILD = true ]]; then
  echo "building cycler"
  go build .. || die 1 "couldn't rebuild cycler"
fi

# Creates a bucket with a randomly named suffix
# (e.g. gs://cycler-integ-test-aeferwhgahgeh).
create_random_bucket() {
  bucket_suffix=$(random_n_chars 32)

  if ! gsutil mb "gs://cycler-integ-test-$bucket_suffix" >/dev/null 2>&1; then
    die 1 "couldn't create bucket gs://cycler-integ-$bucket_suffix"
  fi

  echo "gs://cycler-integ-test-$bucket_suffix"
}

# Creates an object, fills it, copies it to a bucket, deletes temp file.
#
# Takes the following arguments:
# $1 The number of objects.
# $2 The max prefix depth (e.g. gs://one/two/three/four).
# $3 The name of the bucket.
# $4 The count of 512 byte blocks (making up the total filesize).
# $5 The object name prefix
#      (e.g. for "prepre" files like "gs://one/two/prepreobject_name")
create_random_object_in_bucket() {
  for ((i=0; i<$1; i++)); do
    object_path=$(random_object_path "$2" "$3" "$5")
    tmp_file=$(mktemp)
    dd if=/dev/urandom of="$tmp_file" count="$4" >/dev/null 2>&1;
    gsutil cp "$tmp_file" "$object_path" >/dev/null 2>&1;
    rm "$tmp_file"
  done
}

# Cleans up a test by emptying buckets and removing them.
#
# Takes the following arguments:
# $1 The object bucket name.
# $2 The logging bucket name.
# $3 The downloaded logs tmp directory.
# $4 The stdout tmp file.
# $5 The json_out tmp file.
clean_up_test() {
  if [[ $# -ne 5 ]]; then
    die 1 "clean_up_test requires 5 arguments"
  fi

  empty_bucket "$1"
  empty_bucket "$2"

  remove_bucket "$1"
  remove_bucket "$2"

  if [[ "$LEAVELOGS" = false ]]; then
    echo "removing local cycler logs"
    rm -rf "$3"
    rm "$4" "$5"
  fi

  if [[ "$REBUILD" = true ]]; then
    echo "removing built cycler"
    rm ./cycler
  fi
}

# Empties a bucket by using gsutil rm gs://bucket/**
#
# Takes the following arguments:
# $1 The bucket name.
empty_bucket() {
  # If the bucket is already empty the subsequent calls to rm will fail with a
  # command exception: https://github.com/GoogleCloudPlatform/gsutil/issues/417
  if [[ $(gsutil ls "$1" | wc -l) -eq "0" ]]; then
    return
  fi

  if [[ $# -ne 1 ]]; then
    die 1 "empty_bucket requires 2 arguments"
  fi
  if ! gsutil -m rm "$1/**" >/dev/null 2>&1; then
    die 1 "couldn't remove bucket contents $1"
  fi
}

# Removes an empty bucket.
#
# Takes the following arguments:
# $1 The bucket name.
remove_bucket() {
  if [[ $# -ne 1 ]]; then
    die 1 "remove_bucket requires 1 argument"
  fi
  if ! gsutil rb "$1" >/dev/null 2>&1; then
    die 1 "couldn't remove bucket $1"
  fi
}

# Generates a random object path (e.g. dawdwadaw/dawdwadqaw/dwaaw).
#
# Takes the following arguments:
# $1 The max prefix depth.
# $2 The bucket name.
# $3 The last filename prefix.
random_object_path() {
  if [[ $1 -lt 1 ]]; then
    die 1 "need more than 1 prefix"
  fi

  n_prefixes=$((1 + RANDOM % $1))

  object_path=$2
  for ((i=0; i<n_prefixes; i++)); do
    this_suff=$(random_n_chars 12)
    if [[ $i -eq $(( n_prefixes - 1 )) ]]; then
      this_suff=$3$this_suff
    fi
    object_path="$object_path/$this_suff"
  done

  echo "$object_path"
}


# Get n random lowercase chars.
#
# Takes the following arguments:
# $1 The number of chars.
random_n_chars() {
  if [[ $1 -lt 1 ]]; then
    die 1 "need more than 1 char"
  fi

  ( tr -dc '[:lower:]' | fold -w "$1" | head -n 1 ) < /dev/urandom
}

# Decompress the .gz logs recursively in place
#
# Takes the following arguments:
# $1 The .gz log root directory
decompress_logs() {
   # decompress logs in place
  if ! find "$1" -type f -exec gunzip {} \;; then
    echo "couldn't gunzip remote logs"
    false
  else
    true
  fi
}


# Validate that jq can parse the json object in a dir
#
# Takes the following arguments:
# $1 The jsonl root directory
#
# Echos error message if it fails.
validate_jsonl() {
 # validate the logs via jq pass.
  if ! find "$1" -type f -exec \
    sh -c 'jq . "$1" >/dev/null' _ {} \;; then
    echo "cycler logs don't appear to be valid jsonl"
    false
  else
    true
  fi
}

# Validate the number of json objects recursively in dir.
#
# Takes the following arguments:
# $1 The jsonl root directory.
# $2 The number of expected objects.
#
# Echos the error message if it fails
count_jsonl() {
  objects=0
  for f in "$1"/**/*.jsonl; do
    new_objs=$(jq length "$f" | wc -l)
    (( objects=objects+new_objs ))
  done;

  if [[ objects -eq $2 ]]; then
    true
  else
    echo "the number of objects $objects is not the expected $2"
    false
  fi
}

# Validate that the size of the bucket is expected.
#
# Takes the following arguments:
# $1 The expected root size in bytes that cycler will report.
# $2 The json output of cycler.
validate_size() {
  expected_rootsizebytes=$1
  json_out=$2
  actual_rootsizebytes=$(jq '.ActionStats.RootSizeBytes' "$json_out")
  if [[ "$expected_rootsizebytes" -eq "$actual_rootsizebytes" ]]; then
    true
  else
    printf ".PrefixStats.RootSizeByte %s, " "$actual_rootsizebytes"
    printf "expected %s\n" "$expected_rootsizebytes"
    false
  fi
}

# Validate that the count of the objects is expected.
#
# Takes the following arguments:
# $1 The expected count of objects that cycler will report.
# $2 The json output of cycler.
validate_count() {
  expected_file_count=$1
  json_out=$2
  actual_file_count=$(jq '.ActionStats.SizeBytesHistogram.Count' "$json_out")
  if [[ "$expected_file_count" -eq "$actual_file_count" ]]; then
    true
  else
    printf ".SizeBytesHistogram.Count %s, " "$actual_file_count"
    printf "expected %s\n" "$expected_file_count"
    false
  fi
}

# Runs common tests against a cycler run returns failure counts.
#
# This also has the side effect of decompressing the logs.
#
# Takes the following arguments:
# $1 The expected count of objects that cycler will report.
# $2 The expected root size in bytes that cycler will report.
# $3 The logs output of cycler.
# $4 The json output of cycler.
common_checks() {
  local failures=0
  expected_file_count=$1
  expected_rootsizebytes=$2
  logs_out=$3
  json_out=$4

  # make some assertions on the output gathered.
  if ! validate_size "$expected_rootsizebytes" "$json_out"; then
    (( failures++ ))
  fi

  if ! validate_count "$expected_file_count" "$json_out"; then
    (( failures++ ))
  fi

  # decompress logs in place
  if ! decompress_logs "$logs_out"; then
    (( failures++ ))
  fi

  if ! validate_jsonl "$logs_out"; then
    (( failures++ ))
  fi

  if ! count_jsonl "$logs_out" "$expected_file_count"; then
    (( failures++ ))
  fi

  return "$failures"
}
