// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/storage"
	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"
)

// NoopEffect has no actor.

// NoopEffect runtime and configuration state.
type NoopEffect struct {
	Config *cycler_pb.NoopEffectConfiguration `json:"NoopEffectConfiguration"`
}

func (ne NoopEffect) DefaultActor() interface{} {
	return nil
}

// Init nothing.
func (ne *NoopEffect) Initialize(config interface{}, actor interface{}, checks ...bool) {
	return
}

// Enact nothing.
func (ne *NoopEffect) Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error) {
	textResult := fmt.Sprintf("%+v", attr)
	jsonResult, err := json.Marshal(attr)

	if err != nil {
		return nil, fmt.Errorf("Error marshalling json in noopEffect.Enact: %v", err)
	}

	return NoopResult{
		acted:      true,
		jsonResult: string(jsonResult),
		textResult: textResult,
	}, nil
}

// NoopResult defines all outputs of an echo effect.
type NoopResult struct {
	acted      bool
	jsonResult string
	textResult string
}

// HasActed is true if the effect was applied.
func (nr NoopResult) HasActed() bool {
	return nr.acted
}

// JSONResult is the JSON result.
func (nr NoopResult) JSONResult() string {
	return nr.jsonResult
}

// TextResult is the unformatted text result.
func (nr NoopResult) TextResult() string {
	return nr.textResult
}
