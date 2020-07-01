// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

// Duplicate will copy the object into another location (same bucket or another).

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

func (de DuplicateEffect) DefaultActor() interface{} {
	return objectBucketToBucket
}

// DuplicateEffect runtime and configuration state.
type DuplicateEffect struct {
	Config cycler_pb.DuplicateEffectConfiguration `json:"DuplicateEffectConfiguration"`
	// Real or mock actor, non-test invocations use util.objectBucketToBucket.
	actor func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		dstBucket string, prefix string, deleteAfter bool) error
}

// Init the DuplicateEffect, duplicate doesn't mutate so skip checks.
func (de *DuplicateEffect) Initialize(config interface{}, actor interface{}, checks ...bool) {
	orig, ok := config.(cycler_pb.DuplicateEffectConfiguration)
	if !ok {
		log.Printf("Config could not be typecast: %+v", ok)
		os.Exit(2)
	}

	// There is a potential that you overwrite objects using 'duplicate',
	// so consider it an effect that requires mutation to be allowed.
	CheckMutationAllowed(checks)

	de.Config = orig
	de.actor = actor.(func(ctx context.Context, client *storage.Client, srcAttr *storage.ObjectAttrs,
		dstBucket string, prefix string, deleteAfter bool) error)
}

// Enact does the duplicate operation on the attr, does not mutate existing object.
func (de *DuplicateEffect) Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error) {
	err := de.duplicateObject(ctx, client, attr)
	if err != nil {
		return nil, fmt.Errorf("Error duplicating object in DuplicateEffect.enact: %v", err)
	}

	textResult := fmt.Sprintf("%+v", attr)
	jsonResult, err := json.Marshal(attr)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling json in DuplicateEffect.enact: %v", err)
	}
	dr := DuplicateResult{
		acted:      true,
		jsonResult: string(jsonResult),
		textResult: textResult,
	}
	return &dr, nil
}

// Internal duplicate object command for google storage.
func (de *DuplicateEffect) duplicateObject(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) error {
	return de.actor(ctx, client, attr, de.Config.DestinationBucket, de.Config.DestinationPrefix, false)
}

// DuplicateResult defines all outputs of an echo effect.
type DuplicateResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (dr DuplicateResult) HasActed() bool {
	return dr.acted
}

// JSONResult is the JSON result.
func (dr DuplicateResult) JSONResult() string {
	return dr.jsonResult
}

// TextResult is the unformatted text result.
func (dr DuplicateResult) TextResult() string {
	return dr.textResult
}
