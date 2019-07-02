// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package gerrit

import (
	"context"
	"fmt"
	"go.chromium.org/chromiumos/infra/go/internal/shared"
	"go.chromium.org/luci/common/api/gerrit"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"net/http"
	"time"
)

// ChangeRevKey is the necessary set of data for looking up a single Gerrit revision.
type ChangeRevKey struct {
	Host      string
	ChangeNum int64
	Revision  int32
}

func (cik ChangeRevKey) String() string {
	return fmt.Sprintf("%s:%d:%d", cik.Host, cik.ChangeNum, cik.Revision)
}

// ChangeRev contains data about a Gerrit change,revision pair.
type ChangeRev struct {
	ChangeRevKey
	Project string
	Files   []string
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
	ctx, _ = context.WithTimeout(ctx, 5*time.Minute)
	ch := make(chan *gerritpb.ChangeInfo, 1)
	err = shared.DoWithRetry(ctx, shared.DefaultOpts, func() error {
		// This sets the deadline for the individual API call, while the outer context sets
		// an overall timeout for all attempts.
		innerCtx, _ := context.WithTimeout(ctx, 30*time.Second)
		change, err := g.GetChange(innerCtx, &gerritpb.GetChangeRequest{
			Number: changeNum,
			Options: []gerritpb.QueryOption{
				gerritpb.QueryOption_ALL_REVISIONS,
				gerritpb.QueryOption_ALL_FILES,
			}})
		if err != nil {
			return err
		}
		ch <- change
		return nil
	})
	if err != nil {
		return nil, err
	}
	change := <-ch
	for _, v := range change.GetRevisions() {
		if v.Number == revision {
			return &ChangeRev{
				ChangeRevKey: ChangeRevKey{
					Host:      host,
					ChangeNum: change.Number,
					Revision:  v.Number,
				},
				Project: change.Project,
				Files:   getKeys(v.Files),
			}, nil
		}
	}
	return nil, fmt.Errorf("found no revision %d for change %d on host %s", revision, changeNum, host)
}

// ChangeRevData encapsulates a bunch of Gerrit change revisions.
type ChangeRevData struct {
	m map[string]*ChangeRev
}

// GetChangeRev extracts a single Gerrit change revision from the ChangeRevData.
func (crv ChangeRevData) GetChangeRev(host string, changeNum int64, revision int32) (*ChangeRev, error) {
	key := ChangeRevKey{Host: host, ChangeNum: changeNum, Revision: revision}.String()
	val, found := crv.m[key]
	if !found {
		return nil, fmt.Errorf("No ChangeRev found for key %s", key)
	}
	return val, nil
}

// GetChangeRevData fetches the Gerrit changes for the provided ChangeIdKeys, and bundles the result
// into a ChangeRevData.
func GetChangeRevData(authedClient *http.Client, ctx context.Context, changeIds []ChangeRevKey) (*ChangeRevData, error) {
	output := &ChangeRevData{m: make(map[string]*ChangeRev)}
	for _, c := range changeIds {
		if _, exists := output.m[c.String()]; !exists {
			cr, err := GetChangeRev(authedClient, ctx, c.ChangeNum, c.Revision, c.Host)
			if err != nil {
				return output, err
			}
			output.m[c.String()] = cr
		}
	}
	return output, nil
}

// GetChangeRevForTest is intended for testing only, and it allows creation of a ChangeRevData
// through the supplied ChangeRevs.
func GetChangeRevsForTest(cr []*ChangeRev) *ChangeRevData {
	output := &ChangeRevData{m: make(map[string]*ChangeRev)}
	for _, c := range cr {
		output.m[c.String()] = c
	}
	return output
}

// getKeys extracts the keyset from the provided map.
func getKeys(m map[string]*gerritpb.FileInfo) []string {
	keys := make([]string, 0)
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
