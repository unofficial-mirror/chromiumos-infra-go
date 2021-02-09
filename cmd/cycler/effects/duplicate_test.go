// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"
	"google.golang.org/api/option"
	"testing"

	"cloud.google.com/go/storage"
	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"
)

func getDuplicateMock(t *testing.T) interface{} {
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
	config := &cycler_pb.DuplicateEffectConfiguration{
		DestinationBucket: "test_dest",
		DestinationPrefix: "test_prefix",
	}

	ctx := context.Background()
	de := DuplicateEffect{}
	de.Initialize(config, getDuplicateMock(t))

	attr := &storage.ObjectAttrs{}

	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Errorf("couldn't construct client: %v", err)
	}

	duplicateResult, err := de.Enact(ctx, client, attr)
	if err != nil {
		t.Errorf("duplicateResult returned an err:\n%+v", err)
	}
	if duplicateResult.HasActed() != true {
		t.Error("duplicateResult.HasActed() returned false")
	}
}
