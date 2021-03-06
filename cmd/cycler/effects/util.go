// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"
	"fmt"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

// Interbucket copy/move command for google storage, with optional delete.
// prefix is joined added directly to every object name (e.g. 'backup/').
func objectBucketToBucket(ctx context.Context, client *storage.Client,
	srcAttr *storage.ObjectAttrs, dstBucket string, prefix string, deleteAfter bool) error {

	src := client.Bucket(srcAttr.Bucket).Object(srcAttr.Name)
	dst := client.Bucket(dstBucket).Object(prefix + srcAttr.Name)

	if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
		return err
	}
	if deleteAfter {
		return src.Delete(ctx)
	}
	return nil
}

// Change object storage class via copy with src and dst being the same.
func objectChangeStorageClass(ctx context.Context, client *storage.Client,
	srcAttr *storage.ObjectAttrs, toStorageClass cycler_pb.ChillEffectConfiguration_EnumStorageClass) error {

	newStorageClass := cycler_pb.ChillEffectConfiguration_EnumStorageClass.String(toStorageClass)

	// We might not need to change the storage class at all.
	if srcAttr.StorageClass == newStorageClass {
		return nil
	}
	src := client.Bucket(srcAttr.Bucket).Object(srcAttr.Name)
	dst := client.Bucket(srcAttr.Bucket).Object(srcAttr.Name)

	copier := dst.CopierFrom(src)
	copier.StorageClass = newStorageClass

	if _, err := copier.Run(ctx); err != nil {
		return err
	}
	return nil
}

// Delete the provided srtAttr object.
func objectDelete(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs) error {
	if err := client.Bucket(srcAttr.Bucket).Object(srcAttr.Name).Delete(ctx); err != nil {
		return err
	}
	return nil
}

// CheckMutationAllowed will exit if any check in checks is false.
func CheckMutationAllowed(checks []bool) {
	for _, check := range checks {
		if !check {
			fmt.Println("Mutation is not allowed with given configuration.")
			os.Exit(2)
		}
	}
}
