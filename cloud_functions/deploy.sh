#!/usr/bin/env bash
# Copyright 2021 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
(
    echo "Deploying analysis_event_logger"
    cd analysis_event_logger/ || exit
    gcloud functions deploy analysis-event-logger \
           --runtime go113 \
           --trigger-topic "analysis-service-events" \
           --entry-point AnalysisEventLogger
)
