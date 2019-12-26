// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"testing"
)

// TODO(engeg@): This test is not implemented. We need a better way to deal with
// google storage.
func TestMoveEffect(t *testing.T) {
	// config := cycler_pb.MoveEffectConfiguration{
	// 	DestinationBucket: "test_dest",
	// 	DestinationPrefix: "test_prefix",
	// }

	// ctx := context.Background()
	// me := MoveEffect{}
	// me.Init(config)

	// attr := &storage.ObjectAttrs{}

	// client, err := storage.NewClient(ctx)
	// if err != nil {
	// 	t.Error("couldn't construct client")
	// }

	// moveResult, err := me.Enact(ctx, client, attr)
	// if err != nil {
	// 	t.Fail()
	// }

	// if moveResult.HasActed() != true {
	// 	t.Fail()
	// }
}
