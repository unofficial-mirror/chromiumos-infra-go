// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/golang/glog"
)

// AgeInDays returns the age in days from event.
func AgeInDays(event time.Time) (int64, error) {
	age := int64(math.Floor(time.Since(event).Hours() / 24.0))
	if age < 0 {
		return 0, errors.New("object date can't be in the future")
	}
	return age, nil
}

// IntMin returns the minimum of two int64s.
func IntMin(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// StringInSlice returns a bool if a string exists in a slice.
func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// compressBytes gzips an array of bytes into a buffer.
func compressBytes(data *[]byte) (*bytes.Buffer, error) {
	var compressedBytes bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedBytes)
	_, err := gzipWriter.Write(*data)
	if err != nil {
		glog.Errorf("error compressing logs, gzip failed: %v", err)
		return nil, err
	}
	err = gzipWriter.Close()
	if err != nil {
		glog.Errorf("error compressing logs, gzip failed: %v", err)
		return nil, err
	}
	return &compressedBytes, nil
}

// ByteCountSI gives human readable sizes.
func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
