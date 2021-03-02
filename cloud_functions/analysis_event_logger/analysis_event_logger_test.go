// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package cloud_functions

import (
	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/infra/proto/go/analysis_service"
	"testing"
	"time"
)

func TestReadableEventName(t *testing.T) {
	event := &analysis_service.AnalysisServiceEvent{
		Request: &analysis_service.AnalysisServiceEvent_InstallPackagesRequest{},
	}
	if got := readableEventName(event); got != "InstallPackages" {
		t.Errorf("Bad readable name for %s, got %s", "InstallPackages", got)
	}

	event = &analysis_service.AnalysisServiceEvent{
		Request: &analysis_service.AnalysisServiceEvent_BundleRequest{},
	}
	if got := readableEventName(event); got != "Bundle" {
		t.Errorf("Bad readable name for %s, got %s", "Bundle", got)
	}

	event = &analysis_service.AnalysisServiceEvent{
		Request: &analysis_service.AnalysisServiceEvent_BundleVmFilesRequest{},
	}
	if got := readableEventName(event); got != "BundleVmFiles" {
		t.Errorf("Bad readable name for %s, got %s", "BundleVmFiles", got)
	}
}

func TestHivePartionedPath(t *testing.T) {
	dtTime, err := time.Parse("2006-01-02", "2020-02-14")
	dtProto, err := ptypes.TimestampProto(dtTime)
	_ = err

	event := &analysis_service.AnalysisServiceEvent{
		BuildId:     1,
		RequestTime: dtProto,
		Request:     &analysis_service.AnalysisServiceEvent_InstallPackagesRequest{},
	}

	table := "analysis_event_log"
	path := hivePartitionedPath(table, event)
	desired := table + "/dt=2020-02-14/build_id=1/name=InstallPackages/event.json"
	if path != desired {
		t.Errorf("Bad partioned path generated: %s (wanted %s)", path, desired)
	}
}
