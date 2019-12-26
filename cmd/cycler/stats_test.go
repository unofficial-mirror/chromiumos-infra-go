// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

func TestStatsPackage(t *testing.T) {
	ctx := context.Background()
	stats := &Stats{}

	config := DefaultStatsConfiguration()

	stats.init(ctx, &config)

	var sum int64
	for i := 0; i < 10; i++ {
		attr := storage.ObjectAttrs{}
		attr.Size = rand.Int63n(1e+10)
		sum += attr.Size
		attr.Name = uuid.Must(uuid.NewRandom()).String()
		attr.Created = time.Now().AddDate(0, 0, -rand.Intn(1e+3))
		if err := stats.submitUnit(ctx, &attr); err != nil {
			t.Errorf("error on submitUnit: %v", err)
		}
	}

	if err := stats.close(); err != nil {
		t.Errorf("error on close: %v", err)
	}

	// Aside from _getting_ the info, we leave validating the histos to that package.
	if stats.AgeDaysHistogram.Count != 10 {
		t.Fail()
	}

	if stats.SizeBytesHistogram.Sum != sum {
		t.Fail()
	}

	if stats.SizeBytesHistogram.Count != 10 {
		t.Fail()
	}

	if len(stats.textResult()) < 1 {
		t.Fail()
	}

	jsonres, err := stats.jsonResult()
	if err != nil {
		t.Fail()
	}

	if len(jsonres) < 1 {
		t.Fail()
	}

}
