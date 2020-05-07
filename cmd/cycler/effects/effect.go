// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package effects

import (
	"context"

	"cloud.google.com/go/storage"
)

// Effect defines something that a policy might enact on an individual *attr.
type Effect interface {
	// interface{} assumed to be corresponding config struct, checks all must be
	// true to Initialize, and this is used with the mutation allowed parameter at the moment.
	DefaultActor() interface{}
	Initialize(config interface{}, actor interface{}, checks ...bool)
	Enact(ctx context.Context, client *storage.Client, attr *storage.ObjectAttrs) (EffectResult, error)
}

// EffectResult contains the sideproducts of an executed effect.
type EffectResult interface {
	HasActed() bool
	JSONResult() string
	TextResult() string
}
