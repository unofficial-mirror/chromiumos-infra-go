// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package cloud_functions

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/infra/proto/go/analysis_service"
)

// Raw Pub/Sub payload
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// Define a wrapper so we can peek at how many bytes were written through a writer
type CloseWriter interface {
	io.Writer
	io.Closer
}

type CountingWriter struct {
	writer       CloseWriter
	BytesWritten int
}

func (cw *CountingWriter) Close() error {
	return cw.writer.Close()
}

func (cw *CountingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.writer.Write(p)
	cw.BytesWritten += n
	return n, err
}

// Generate a readable name from an event.
func readableEventName(msg *analysis_service.AnalysisServiceEvent) string {
	request_name := reflect.ValueOf(msg.Request).Type().Elem().Name()

	// Protobuffer oneof types are prefixed with their parent message type,
	// so let's strip that off
	request_name = strings.ReplaceAll(request_name, "AnalysisServiceEvent_", "")

	// And remove "Request" part of the name since our message will contain
	// the request _and_ response
	return strings.ReplaceAll(request_name, "Request", "")
}

// Generate a path for a given AnalysisServiceEvent with Hive partitioning
//   see https://cloud.google.com/bigquery/docs/hive-partitioned-queries-gcs
// for more information.
func hivePartitionedPath(gcpTable string, event *analysis_service.AnalysisServiceEvent) string {
	timestamp, err := ptypes.Timestamp(event.RequestTime)
	if err != nil {
		log.Fatalf("Error decoding timestamp from message: %v", err)
	}

	return fmt.Sprintf("%s/dt=%s/build_id=%d/name=%s/event.json",
		gcpTable,
		timestamp.Format("2006-01-02"),
		event.BuildId,
		readableEventName(event))
}

// Main entry point for the Cloud Function.  Takes a PubSubMessage, decodes it
// to an AnalysisServiceEvent, then writes it to a partitioned path as a
// compressed json message.
func AnalysisEventLogger(ctx context.Context, msg PubSubMessage) error {
	gcpBucket := "chromeos-cdp-analytics"
	gcpTable := "analysis_event_log"

	// Decode binary proto message
	analysisEvent := &analysis_service.AnalysisServiceEvent{}
	if err := proto.Unmarshal(msg.Data, analysisEvent); err != nil {
		log.Fatalf("Error unmarshalling message: %v", err)
	}

	// Generate a properly partitioned path name for the event
	object_name := hivePartitionedPath(gcpTable, analysisEvent)

	// Create storage client and bucket/object handles
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Could not create storage client: %v", err)
	}
	bucket := client.Bucket(gcpBucket)
	object := bucket.Object(object_name)

	// Create a writer into the object, set headers to identify it as compressed
	objWriter := object.NewWriter(ctx)
	objWriter.ContentType = "application/x-json-stream"
	objWriter.ContentEncoding = "gzip"

	// Wrap up writers so we can sniff the written byte count
	writer := &CountingWriter{writer: objWriter}
	zipper := &CountingWriter{writer: gzip.NewWriter(writer)}

	marshaler := jsonpb.Marshaler{}
	if err := marshaler.Marshal(zipper, analysisEvent); err != nil {
		log.Fatalf("Error writing message: %v", err)
	}

	if err := zipper.Close(); err != nil {
		log.Fatalf("Error closing zipper: %v", err)
	}

	if err := writer.Close(); err != nil {
		log.Fatalf("Error closing writer: %v", err)
	}

	log.Printf("Wrote object %s (size: %d bytes, compressed: %d bytes, ratio: %.2f)",
		object_name, zipper.BytesWritten, writer.BytesWritten,
		float64(writer.BytesWritten)/float64(zipper.BytesWritten))

	return nil
}
