// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package util

import (
	"testing"

	"gotest.tools/assert"
)

func TestUnorderedEqual(t *testing.T) {
	a := []string{"a", "b", "c", "a"}
	b := []string{"b", "c", "a", "a"}
	c := []string{"a", "b", "b", "c"}
	assert.Assert(t, UnorderedEqual(a, b))
	assert.Assert(t, !UnorderedEqual(a, c))
}

func TestUnorderedContains(t *testing.T) {
	a := []string{"a", "b", "c", "a"}
	b := []string{"b", "c"}
	c := []string{"b", "d"}
	assert.Assert(t, UnorderedContains(a, b))
	assert.Assert(t, !UnorderedContains(a, c))
}
