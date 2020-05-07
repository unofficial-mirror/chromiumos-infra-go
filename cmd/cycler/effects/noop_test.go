// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
)

func TestNoopEffect(t *testing.T) {
	attr := &storage.ObjectAttrs{}
	ne := NoopEffect{}
	ne.Initialize(nil, nil)

	// We can 'get away' with passing nil here as we expect, nay, _demand_,
	// that the noop action doesn't use a GS client.
	_, err := ne.Enact(context.Background(), nil, attr)
	if err != nil {
		t.Fail()
	}
}
