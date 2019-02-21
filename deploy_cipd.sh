#!/bin/bash -e

# Copyright 2019 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "$(dirname "$0")"

# Wrap all testplan commands as cipd package.
CMDPATH="./src/testplans/cmd"
OUTPATH="$(pwd -P)/.out"

if [ -e $OUTPATH ]; then
  rm -r $OUTPATH/*
fi

# translation:
# all cmds | does not start with "dev_" | strip trailing /
COMMANDS=$(cd $CMDPATH; ls -d */ | grep "^[^dev_]" | sed 's#/##')

for go_cmd in $COMMANDS; do
  (cd "$CMDPATH/$go_cmd" && go build -o "$OUTPATH/$go_cmd")
done

cipd create -pkg-def=cipd.yaml -ref latest -json-output deploy_cipd.json
