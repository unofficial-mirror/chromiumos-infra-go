// Copyright 2019 The Chromium OS Authors. All rights reserved.
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

func getMoveMock(t *testing.T) interface{} {
	return func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		dstBucket string, prefix string, deleteAfter bool) error {
		if deleteAfter == false {
			t.Errorf("Move must call with 'deleteAfter'")
		}
		if dstBucket != "test_dest" || prefix != "test_prefix" {
			t.Errorf("Actor called with differing bucket parameters")
		}
		return nil
	}
}

func TestMoveEffect(t *testing.T) {
	config := &cycler_pb.MoveEffectConfiguration{
		DestinationBucket: "test_dest",
		DestinationPrefix: "test_prefix",
	}

	ctx := context.Background()
	me := MoveEffect{}
	me.Initialize(config, getMoveMock(t))

	attr := &storage.ObjectAttrs{}

	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Errorf("couldn't construct client: %v", err)
	}

	moveResult, err := me.Enact(ctx, client, attr)
	if err != nil {
		t.Errorf("moveResult returned an err:\n%+v", err)
	}

	if moveResult.HasActed() != true {
		t.Error("moveResult.HasActed() returned false")
	}
}
