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

# Initializes the options for the test harness script.
while getopts "vdx" o; do
  case "${o}" in
    v)
      VERBOSE=true
      echo "verbose mode: $VERBOSE"
      ;;
    x)
      SETX=true
      echo "setting -x (extremely verbose)"
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
# $2 The max prefix depth (i.e. gs://one/two/three/four).
# $3 The name of the bucket.
# $4 The count of 512 byte blocks (making up the total filesize).
create_random_object_in_bucket() {
  for ((i=0; i<$1; i++)); do
    object_path=$(random_object_path "$2" "$3")
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
clean_up_test() {
  if [[ $# -ne 2 ]]; then
    die 1 "clean_up_test requires 2 arguments"
  fi

  empty_bucket "$1"
  empty_bucket "$2"

  remove_bucket "$1"
  remove_bucket "$2"

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
random_object_path() {
  if [[ $1 -lt 1 ]]; then
    die 1 "need more than 1 prefix"
  fi

  n_prefixes=$((1 + RANDOM % $1))

  object_path=$2
  for ((i=0; i<n_prefixes; i++)); do
    this_suff=$(random_n_chars 12)
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


