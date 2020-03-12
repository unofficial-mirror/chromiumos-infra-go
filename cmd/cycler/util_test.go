// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"testing"
	"time"
)

func TestAgeInDays(t *testing.T) {
	age, err := AgeInDays(time.Now())
	if age != 0 || err != nil {
		t.Fail()
	}

	age, err = AgeInDays(time.Now().Add(time.Hour * -24))
	if age != 1 || err != nil {
		t.Fail()
	}
	age, err = AgeInDays(time.Now().Add(time.Hour * 24 * -23))
	if age != 23 || err != nil {
		t.Fail()
	}
}

func TestIntMin(t *testing.T) {
	min := IntMin(0, 1)
	if min != 0 {
		t.Fail()
	}

	min = IntMin(3, 3)
	if min != 3 {
		t.Fail()
	}

	min = IntMin(-1, 4)
	if min != -1 {
		t.Fail()
	}
}

func TestStringInSlice(t *testing.T) {
	t1 := []string{"a", "bunch", "of", "test", "strings"}
	if !StringInSlice("test", t1) {
		t.Fail()
	}
	if StringInSlice("hoobaloo", t1) {
		t.Fail()
	}
	if StringInSlice("string", t1) {
		t.Fail()
	}
}

func TestCompressBytes(t *testing.T) {
	data := []byte{0}

	// This might be fragile depending on the internals of gzip. If it's failing
	// but cycler\compression appears to be working suspect this.
	validationData := []byte{31, 139, 8, 0, 0, 0, 0, 0, 0, 255, 98, 0, 4, 0, 0,
		255, 255, 141, 239, 2, 210, 1, 0, 0, 0}

	compData, err := compressBytes(&data)

	if err != nil {
		t.Fail()
	}

	if bytes.Compare(compData.Bytes(), validationData) != 0 {
		t.Errorf("compressed bytes differ:\nExpected: %v\nActual:%v", validationData, compData)
	}
}

func TestByteCountSI(t *testing.T) {
	str := ByteCountSI(1)
	if "1 B" != str {
		t.Fail()
	}
	str = ByteCountSI(32)
	if "32 B" != str {
		t.Fail()
	}
	str = ByteCountSI(1048576)
	if "1.0 MB" != str {
		t.Fail()
	}

	// A little frustrating that this function doesn't give a round 1.0 here,
	// but it's just display for casual consumption and is "close enough™"
	str = ByteCountSI(1073741824)
	if "1.1 GB" != str {
		t.Fail()
	}
}
