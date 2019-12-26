// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

// Move will move the object into another location (same bucket or another).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

// MoveEffect runtime and configuration state.
type MoveEffect struct {
	Config cycler_pb.MoveEffectConfiguration `json:"MoveEffectConfiguration"`
}

// MoveEffectConfig has no configuration.
type MoveEffectConfig struct {
	DestinationBucket string `json:"DestinationBucket"`
	DestinationPrefix string `json:"DestinationPrefix"`
}

// Init the move effect.
func (me *MoveEffect) Init(config interface{}, checks ...bool) {
	orig, ok := config.(cycler_pb.MoveEffectConfiguration)
	if !ok {
		fmt.Fprintf(os.Stderr, "Config could not be typecast: %+v", ok)
		os.Exit(2)
	}

	CheckMutationAllowed(checks)

	me.Config = orig
}

// Enact does the move operation on the attr, _this deletes the old object_!
func (me *MoveEffect) Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error) {
	err := me.moveObject(ctx, client, attr)

	if err != nil {
		return nil, fmt.Errorf("Error moving object (%v) in moveEffect.Enact: %v", attr.Name, err)
	}

	textResult := fmt.Sprintf("%+v", attr)
	jsonResult, err := json.Marshal(attr)

	if err != nil {
		return nil, fmt.Errorf("Error marshalling json in moveEffect.Enact: %v", err)
	}

	er := MoveResult{
		acted:      true,
		jsonResult: string(jsonResult),
		textResult: textResult,
	}

	return &er, nil
}

// Internal move object command for google storage.
func (me *MoveEffect) moveObject(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) error {
	return objectBucketToBucket(ctx, client, attr, me.Config.DestinationBucket, me.Config.DestinationPrefix, true)
}

// MoveResult defines all outputs of a move effect.
type MoveResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (er *MoveResult) HasActed() bool {
	return er.acted
}

// JSONResult is the JSON result.
func (er *MoveResult) JSONResult() string {
	return er.jsonResult
}

// TextResult is the unformatted text result.
func (er *MoveResult) TextResult() string {
	return er.textResult
}
