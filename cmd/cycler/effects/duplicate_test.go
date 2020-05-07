// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"
)

func getDuplicateMock(t *testing.T) DuplicateEffectActor {
	return func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		dstBucket string, prefix string, deleteAfter bool) error {
		if deleteAfter == true {
			t.Errorf("Duplicate must not call with 'deleteAfter'")
		}
		if dstBucket != "test_dest" || prefix != "test_prefix" {
			t.Errorf("Actor called with differing bucket parameters")
		}
		return nil
	}
}

func TestDuplicateEffect(t *testing.T) {
	config := cycler_pb.DuplicateEffectConfiguration{
		DestinationBucket: "test_dest",
		DestinationPrefix: "test_prefix",
	}

	ctx := context.Background()
	de := DuplicateEffect{}
	de.Initialize(config, getDuplicateMock(t))

	attr := &storage.ObjectAttrs{}

	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Error("couldn't construct client")
	}

	moveResult, err := de.Enact(ctx, client, attr)
	if err != nil {
		t.Fail()
	}

	if moveResult.HasActed() != true {
		t.Fail()
	}
}
