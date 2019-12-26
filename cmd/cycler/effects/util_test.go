// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"testing"
)

// TODO(engeg@): Currently no tests of the objectBucketToBucket.

// This will Exit(2) on failure.
func TestCheckMutationAllowed(t *testing.T) {
	CheckMutationAllowed([]bool{true, true, true})
}
