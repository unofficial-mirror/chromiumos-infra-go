// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
The stats action calculates various stats on the objects within
the bucket and outputs results. This action should encompass anything
in which we have retrieved *storage.ObjectAttrs and nothing _too_ specific
to do with it (i.e. if we're pulling that struct down from GS and aren't
mutating, but simply aggregating, we should just fold it into this op).

Currently the following metrics aggregations are supported:
	* Total size
	* Size by prefix
	* Object age histogram.
	* Object size histogram.

Many more are possible (acls, etc.) and we expect to add then as needed.
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
	"google.golang.org/grpc/benchmark/stats"
)

// Stats is the collection of aggregated and raw statistics. This
// is the internal runtime struct.
type Stats struct {
	// Total sum of bytes.
	RootSizeBytes int64 `json:"RootSizeBytes"`

	// For an object like gs://bucket/dir1/dir2/objectName we will
	// add entries for bucket, bucket/dir1 and bucket/dir1/dir2.
	PrefixMapSizeBytes map[string]int64 `json:"PrefixMapSizeBytes"`

	// Object age histogram.
	AgeDaysHistogram stats.Histogram `json:"AgeDaysHistogram"`

	// Object size in bytes histogram.
	SizeBytesHistogram stats.Histogram `json:"SizeBytesHistogram"`

	// General config.
	Config *cycler_pb.StatsConfiguration `json:"StatsConfiguration"`

	// Used to protect non-thread-safe members.
	mux sync.Mutex
}

// DefaultStatsConfiguration returns a default configuration for StatsConfiguration.
func DefaultStatsConfiguration() *cycler_pb.StatsConfiguration {
	return &cycler_pb.StatsConfiguration{
		// Max depth is default to '1' to avoid massive report sizes.
		PrefixReportMaxDepth: 1,
		AgeDaysHistogramOptions: &cycler_pb.HistogramOptions{
			NumBuckets:     13,
			GrowthFactor:   1.0,
			BaseBucketSize: 1,
			MinValue:       0,
		},
		SizeBytesHistogramOptions: &cycler_pb.HistogramOptions{
			NumBuckets:     28,
			GrowthFactor:   1.0,
			BaseBucketSize: 512,
			MinValue:       0,
		},
	}
}

// init the config object.
func (s *Stats) init(ctx context.Context, config *cycler_pb.StatsConfiguration) {

	if config == nil {
		s.Config = DefaultStatsConfiguration()
	} else {
		s.Config = config
	}

	s.AgeDaysHistogram = *stats.NewHistogram(convertHistogramOptions(s.Config.AgeDaysHistogramOptions))
	s.SizeBytesHistogram = *stats.NewHistogram(convertHistogramOptions(s.Config.SizeBytesHistogramOptions))
	s.PrefixMapSizeBytes = make(map[string]int64)
}

// submitUnit submits a single ObjectAttr to the histogram stats logic.
func (s *Stats) submitUnit(ctx context.Context, attr *storage.ObjectAttrs) error {
	defer func() {
		s.mux.Unlock()
	}()
	s.mux.Lock()

	// Update prefix size map.
	// We start len-1 because len is the name of the object itself.
	splits := strings.Split(attr.Name, "/")
	depth := IntMin(len(splits)-1, int(s.Config.PrefixReportMaxDepth))
	for i := depth; i > 0; i-- {
		// Join up splits until i and increment prefixMapSizeBytes.
		index := strings.Join(splits[0:i], "/")
		s.PrefixMapSizeBytes[index] += attr.Size
	}

	// Update the object age histogram.
	age, err := AgeInDays(attr.Created)
	if err != nil {
		return errors.New("couldn't convert age to days")
	}

	if err := s.AgeDaysHistogram.Add(age); err != nil {
		return fmt.Errorf("couldn't add to age histogram: %v", err)
	}

	// Update the object size histogram.
	if err := s.SizeBytesHistogram.Add(attr.Size); err != nil {
		return fmt.Errorf("couldn't add to size histogram: %v", err)
	}

	atomic.AddInt64(&s.RootSizeBytes, (*attr).Size)
	return nil
}

// close finalizes Stats.
func (s *Stats) close() error {
	return nil
}

// result returns the json marshalled Stats.
func (s *Stats) jsonResult() ([]byte, error) {
	return json.Marshal(s)
}

// textResult returns a text representation of the results.
func (s *Stats) textResult() string {
	str := ""

	str += "\nObject created age (days) histogram:\n"
	buf := new(bytes.Buffer)
	s.AgeDaysHistogram.Print(buf)
	str += buf.String()

	str += "\nObject size (bytes) histogram:\n"
	buf = new(bytes.Buffer)
	s.SizeBytesHistogram.Print(buf)
	str += buf.String()

	str += "\nPath prefixed total sizes:\n"
	keys := make([]string, len(s.PrefixMapSizeBytes))
	i := 0
	for k := range s.PrefixMapSizeBytes {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	for _, k := range keys {
		str += fmt.Sprintf("%v %v\n", s.PrefixMapSizeBytes[k], k)
	}

	str += fmt.Sprintf("\nTotal size of all objects: %v\n", ByteCountSI(s.RootSizeBytes))
	return str
}

func convertHistogramOptions(ho *cycler_pb.HistogramOptions) stats.HistogramOptions {
	return stats.HistogramOptions{
		NumBuckets:     int(ho.NumBuckets),
		GrowthFactor:   float64(ho.GrowthFactor),
		BaseBucketSize: float64(ho.BaseBucketSize),
		MinValue:       ho.MinValue,
	}
}
