// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"context"
	"fmt"
	"go.chromium.org/luci/common/api/gerrit"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"net/http"
)

// ChangeRev contains data about a Gerrit change,revision pair.
type ChangeRev struct {
	ChangeNum int64
	Project   string
	Revision  int32
	Files     []string
}

var (
	// Override this to use a mock GerritClient rather than the real one.
	mockGerrit gerritpb.GerritClient
)

// GetChangeRev gets details from Gerrit about a change,revision pair.
func GetChangeRev(authedClient *http.Client, ctx context.Context, changeNum int64, revision int32, host string) (*ChangeRev, error) {
	var g gerritpb.GerritClient
	var err error
	if mockGerrit != nil {
		g = mockGerrit
	} else {
		if g, err = gerrit.NewRESTClient(authedClient, host, true); err != nil {
			return nil, err
		}
	}
	ch, err := g.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number: changeNum,
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_ALL_REVISIONS,
			gerritpb.QueryOption_ALL_FILES,
		}})
	if err != nil {
		return nil, err
	}
	for _, v := range ch.GetRevisions() {
		if v.Number == revision {
			return &ChangeRev{
				ChangeNum: ch.Number,
				Project:   ch.Project,
				Revision:  v.Number,
				Files:     getKeys(v.Files),
			}, nil
		}
	}
	return nil, fmt.Errorf("found no revision %d for change %d on host %s", revision, changeNum, host)
}

func getKeys(m map[string]*gerritpb.FileInfo) []string {
	keys := make([]string, 0)
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
