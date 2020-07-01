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

func getDeleteMock(t *testing.T) interface{} {
	return func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs) error {
		if srcAttr.Bucket != "test_bucket" || srcAttr.Name != "test_object.txt" {
			t.Error("The bucket or object name did not match configured.")
		}
		return nil
	}
}

func TestDeleteEffect(t *testing.T) {
	config := cycler_pb.DeleteEffectConfiguration{}

	ctx := context.Background()
	de := DeleteEffect{}
	de.Initialize(config, getDeleteMock(t))

	attr := &storage.ObjectAttrs{
		Bucket: "test_bucket",
		Name:   "test_object.txt",
	}

	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Errorf("couldn't construct client: %v", err)
	}

	deleteResult, err := de.Enact(ctx, client, attr)
	if err != nil {
		t.Errorf("deleteResult returned an err:\n%+v", err)
	}

	if deleteResult.HasActed() != true {
		t.Error("deleteResult.HasActed() returned false")
	}
}
