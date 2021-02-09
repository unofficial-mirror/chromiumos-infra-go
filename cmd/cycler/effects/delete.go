// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

// Delete _immediately deletes_ the object.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

func (de DeleteEffect) DefaultActor() interface{} {
	return objectDelete
}

// DeleteEffect runtime and configuration state.
type DeleteEffect struct {
	Config *cycler_pb.DeleteEffectConfiguration `json:"DeleteEffectConfiguration"`
	// Real or mock actor, non-test invocations use util.objectBucketToBucket.
	actor func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs) error
}

// Init the DeleteEffect
func (de *DeleteEffect) Initialize(config interface{}, actor interface{}, checks ...bool) {
	orig, ok := config.(*cycler_pb.DeleteEffectConfiguration)
	if !ok {
		log.Printf("Config could not be typecast: %+v", ok)
		os.Exit(2)
	}

	// Delete obviously requires mutation to be allowed.
	CheckMutationAllowed(checks)

	de.Config = orig
	de.actor = actor.(func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs) error)
}

// Enact does the delete operation on the attr.
func (de *DeleteEffect) Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error) {
	err := de.deleteObject(ctx, client, attr)
	if err != nil {
		return nil, fmt.Errorf("Error deleting in DeleteEffect.enact: %v", err)
	}

	textResult := fmt.Sprintf("%+v", attr)
	jsonResult, err := json.Marshal(attr)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling json in DeleteEffect.enact: %v", err)
	}
	dr := DeleteResult{
		acted:      true,
		jsonResult: string(jsonResult),
		textResult: textResult,
	}
	return &dr, nil
}

// Internal delete object command for google storage.
func (de *DeleteEffect) deleteObject(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) error {
	return de.actor(ctx, client, attr)
}

// DeleteResult defines all outputs of a delet eeffect.
type DeleteResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (dr DeleteResult) HasActed() bool {
	return dr.acted
}

// JSONResult is the JSON result.
func (dr DeleteResult) JSONResult() string {
	return dr.jsonResult
}

// TextResult is the unformatted text result.
func (dr DeleteResult) TextResult() string {
	return dr.textResult
}
