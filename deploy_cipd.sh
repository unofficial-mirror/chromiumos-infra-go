#!/bin/bash -e

# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "$(dirname "$0")"

# Wrap all testplan commands as cipd package.
CMDPATH="./cmd"
OUTPATH="$(pwd -P)/.out"

if [ -e $OUTPATH ]; then
  rm -r $OUTPATH/*
fi

# translation:
# all cmds | strip trailing /
COMMANDS=$(cd $CMDPATH; ls -d */ | sed 's#/##')

for go_cmd in $COMMANDS; do
  # Target linux amd64, as that's what the GCE bots use.
  (cd "$CMDPATH/$go_cmd" && GOOS=linux GOARCH=amd64 go build -o "$OUTPATH/$go_cmd")
done

GIT_REVISION=$(git rev-parse HEAD)

cipd create -pkg-def=cipd.yaml -ref latest -json-output deploy_cipd.json \
  -tag "git_revision:${GIT_REVISION}"
