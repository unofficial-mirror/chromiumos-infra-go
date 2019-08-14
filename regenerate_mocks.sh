#!/bin/bash
# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Move to this script's directory.
cd "$(dirname "$0")"

# List of files to generate mocks from.
# Mocked files are placed in a subdirectory mock/ in the mocked file's
# directory.
# For example, if you mocked a/b/c.go, it would be placed in a/b/mock/c.go.
# If the mock/ subdirectory does not exist, it will be created.
mocks=(
)

license="// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.\n"

for i in "${mocks[@]}"; do
  # Try to create mock/ subdirectory in case it doesn't exist.
  mkdir "$(dirname $i)/mock" 2>/dev/null
  # Generate mock file
  dest="$(dirname $i)/mock/$(basename $i)"
  echo "generating $dest"
  mockgen -source "$i" > "$dest"
  # Prepend license to mocked file
  echo -e "$license$(cat $dest)" > "$dest"
done
