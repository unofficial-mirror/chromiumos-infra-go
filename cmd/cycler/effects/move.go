// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

// Move will move the object into another location (same bucket or another).

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

func (me MoveEffect) DefaultActor() interface{} {
	return objectBucketToBucket
}

// MoveEffect runtime and configuration state.
type MoveEffect struct {
	Config cycler_pb.MoveEffectConfiguration `json:"MoveEffectConfiguration"`
	// Real or mock actor, non-test invocations use util.objectBucketToBucket.
	actor func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		dstBucket string, prefix string, deleteAfter bool) error
}

// MoveEffectConfig configuration.
type MoveEffectConfig struct {
	DestinationBucket string `json:"DestinationBucket"`
	DestinationPrefix string `json:"DestinationPrefix"`
}

// Init the move effect with a config and an actor (mock or real function).
func (me *MoveEffect) Initialize(config interface{}, actor interface{}, checks ...bool) {
	orig, ok := config.(cycler_pb.MoveEffectConfiguration)
	if !ok {
		log.Printf("Config could not be typecast: %+v", ok)
		os.Exit(2)
	}

	CheckMutationAllowed(checks)

	me.Config = orig
	me.actor = actor.(func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		dstBucket string, prefix string, deleteAfter bool) error)
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
	return me.actor(ctx, client, attr, me.Config.DestinationBucket, me.Config.DestinationPrefix, true)
}

// MoveResult defines all outputs of a move effect.
type MoveResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (mr MoveResult) HasActed() bool {
	return mr.acted
}

// JSONResult is the JSON result.
func (mr MoveResult) JSONResult() string {
	return mr.jsonResult
}

// TextResult is the unformatted text result.
func (mr MoveResult) TextResult() string {
	return mr.textResult
}
