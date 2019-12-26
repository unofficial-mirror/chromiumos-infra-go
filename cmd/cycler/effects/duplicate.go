// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

// Duplicate will copy the object into another location (same bucket or another).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
)

// DuplicateEffect runtime and configuration state.
type DuplicateEffect struct {
	Config cycler_pb.DuplicateEffectConfiguration `json:"DuplicateEffectConfiguration"`
}

// Init the DuplicateEffect, duplicate doesn't mutate so skip checks.
func (me *DuplicateEffect) Init(config interface{}, checks ...bool) {
	orig, ok := config.(cycler_pb.DuplicateEffectConfiguration)
	if !ok {
		fmt.Fprintf(os.Stderr, "Config could not be typecast: %+v", ok)
		os.Exit(2)
	}

	// There is a potential that you overwrite objects using 'duplicate',
	// so consider it an effect that requires mutation to be allowed.
	CheckMutationAllowed(checks)

	me.Config = orig
}

// Enact does the duplicate operation on the attr, does not mutate existing object.
func (me *DuplicateEffect) Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error) {
	err := me.duplicateObject(ctx, client, attr)
	if err != nil {
		return nil, fmt.Errorf("Error duplicating object in DuplicateEffect.enact: %v", err)
	}

	textResult := fmt.Sprintf("%+v", attr)
	jsonResult, err := json.Marshal(attr)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling json in DuplicateEffect.enact: %v", err)
	}
	er := DuplicateResult{
		acted:      true,
		jsonResult: string(jsonResult),
		textResult: textResult,
	}
	return &er, nil
}

// Internal duplicate object command for google storage.
func (me *DuplicateEffect) duplicateObject(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) error {
	return objectBucketToBucket(ctx, client, attr, me.Config.DestinationBucket, me.Config.DestinationPrefix, false)
}

// DuplicateResult defines all outputs of an echo effect.
type DuplicateResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (er *DuplicateResult) HasActed() bool {
	return er.acted
}

// JSONResult is the JSON result.
func (er *DuplicateResult) JSONResult() string {
	return er.jsonResult
}

// TextResult is the unformatted text result.
func (er *DuplicateResult) TextResult() string {
	return er.textResult
}
