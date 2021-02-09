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

func getChillMock(t *testing.T) interface{} {
	return func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		toStorageClass cycler_pb.ChillEffectConfiguration_EnumStorageClass) error {
		if toStorageClass != cycler_pb.ChillEffectConfiguration_COLDLINE {
			t.Errorf("Wanted COLDLINE, got %+v", toStorageClass)
		}
		return nil
	}
}

func TestChillEffect(t *testing.T) {
	config := &cycler_pb.ChillEffectConfiguration{
		ToStorageClass: cycler_pb.ChillEffectConfiguration_COLDLINE,
	}

	ctx := context.Background()
	ce := ChillEffect{}
	ce.Initialize(config, getChillMock(t))

	attr := &storage.ObjectAttrs{}

	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Errorf("couldn't construct client: %v", err)
	}

	chillResult, err := ce.Enact(ctx, client, attr)
	if err != nil {
		t.Errorf("chillResult returned an err:\n%+v", err)
	}

	if chillResult.HasActed() != true {
		t.Error("chillResult.HasActed() returned false")
	}
}

//TODO(engeg@) Add test we never call actor if storage class doesn't change.
