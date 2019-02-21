#!/bin/bash -e
# Copyright 2019 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd "$(dirname "$0")"

# Fetch the latest lucicfg to go.mod. This is necessary in case
go get -u go.chromium.org/luci

luci_path=$(go mod download -json | grep "go.chromium.org/luci" | grep \"Dir\" | grep -o '[/][^"]*')
echo "Found luci_path: ${luci_path}"

protoc --proto_path=. --proto_path=${luci_path} --go_out . *.proto
echo "Successfully regenerated protos"