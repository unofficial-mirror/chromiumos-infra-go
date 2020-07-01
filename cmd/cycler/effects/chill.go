// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Chill changes the object class (ostensibly from a higher to lower one).
// See: https://cloud.google.com/storage/docs/storage-classes

package effects

// Change the storage class of an object in place.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

func (ce ChillEffect) DefaultActor() interface{} {
	return objectChangeStorageClass
}

// ChillEffect runtime and configuration state.
type ChillEffect struct {
	Config cycler_pb.ChillEffectConfiguration `json:"ChillEffectConfiguration"`
	// Real or mock actor, non-test invocations use util.objectChangeStorageClass
	actor func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		toStorageClass cycler_pb.ChillEffectConfiguration_EnumStorageClass) error
}

// Init the chill effect.
func (ce *ChillEffect) Initialize(config interface{}, actor interface{}, checks ...bool) {
	orig, ok := config.(cycler_pb.ChillEffectConfiguration)
	if !ok {
		log.Printf("Config could not be typecast: %+v", ok)
		os.Exit(2)
	}

	// Validate the configuration.
	if orig.ToStorageClass == cycler_pb.ChillEffectConfiguration_UNKNOWN {
		log.Printf("UNKNOWN is not a valid storage class.")
		os.Exit(2)
	}

	CheckMutationAllowed(checks)

	ce.Config = orig
	ce.actor = actor.(func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		toStorageClass cycler_pb.ChillEffectConfiguration_EnumStorageClass) error)

}

// Enact does the move operation on the attr, _this deletes the old object_!
func (ce *ChillEffect) Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error) {
	err := ce.chillObject(ctx, client, attr)

	if err != nil {
		return nil, fmt.Errorf("Error chilling object (%v) in chillEffect.Enact: %v", attr.Name, err)
	}

	textResult := fmt.Sprintf("%+v", attr)
	jsonResult, err := json.Marshal(attr)

	if err != nil {
		return nil, fmt.Errorf("Error marshalling json in chillEffect.Enact: %v", err)
	}

	cr := ChillResult{
		acted:      true,
		jsonResult: string(jsonResult),
		textResult: textResult,
	}

	return &cr, nil
}

// Internal copy object command for google storage.
func (ce *ChillEffect) chillObject(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) error {
	return ce.actor(ctx, client, attr, ce.Config.ToStorageClass)
}

// ChillResult defines all outputs of a move effect.
type ChillResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (cr ChillResult) HasActed() bool {
	return cr.acted
}

// JSONResult is the JSON result.
func (cr ChillResult) JSONResult() string {
	return cr.jsonResult
}

// TextResult is the unformatted text result.
func (cr ChillResult) TextResult() string {
	return cr.textResult
}
