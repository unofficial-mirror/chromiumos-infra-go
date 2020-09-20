// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package generator

import (
	"sort"

	"go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

var (
	// boardPriorities is a map of Skylab boards to their relative oversupply in
	// the lab, compared to their demand. A board that never sees pending times
	// should get a very negative number. A board with large demand should get
	// a large positive number. Any unlisted board is implicitly 0.
	// This may be moved into config at some point soon.
	boardPriorities = map[string]int32{
		"coral":  -20,
		"sarien": 2,
		"eve":    5,
	}
)

type testSuite struct {
	skylabBoard string
	build       bbproto.Build
	tsc         *testplans.TestSuiteCommon
	isVm        bool
}

type group string

func tsc(buildResult buildResult, skylabBoard string, tsc *testplans.TestSuiteCommon, isVm bool, m map[group][]testSuite) map[group][]testSuite {
	ts := testSuite{
		tsc:         tsc,
		build:       buildResult.build,
		skylabBoard: skylabBoard,
		isVm:        isVm,
	}
	for _, tsg := range tsc.GetTestSuiteGroups() {
		g := group(tsg.GetTestSuiteGroup())
		m[g] = append(m[g], ts)
	}
	return m
}

func (ts testSuite) String() string {
	return ts.tsc.GetDisplayName()
}

// groupAndSort groups known test suites by the test group(s) they're in, then
// sorts each group by the preference that the test plan generator should show
// toward elements in that group. The first element in each group is the one
// that the test plan generator is encouraged to schedule against first.
// This all supports oneof-based testing. See go/cq-oneof
func groupAndSort(buildResult []buildResult) map[group][]testSuite {
	m := make(map[group][]testSuite)
	for _, br := range buildResult {
		req := br.perTargetTestReqs
		for _, t := range req.GetHwTestCfg().GetHwTest() {
			m = tsc(br, t.GetSkylabBoard(), t.GetCommon(), false, m)
		}
		for _, t := range req.GetDirectTastVmTestCfg().GetTastVmTest() {
			m = tsc(br, br.buildId.buildTarget, t.GetCommon(), true, m)
		}
		for _, t := range req.GetMoblabVmTestCfg().GetMoblabTest() {
			m = tsc(br, br.buildId.buildTarget, t.GetCommon(), true, m)
		}
		for _, t := range req.GetTastVmTestCfg().GetTastVmTest() {
			m = tsc(br, br.buildId.buildTarget, t.GetCommon(), true, m)
		}
		for _, t := range req.GetVmTestCfg().GetVmTest() {
			m = tsc(br, br.buildId.buildTarget, t.GetCommon(), true, m)
		}
	}
	for _, suites := range m {
		sort.Slice(suites, func(i, j int) bool {
			if suites[i].tsc.GetCritical().GetValue() != suites[j].tsc.GetCritical().GetValue() {
				// critical test suites at the front
				return suites[i].tsc.GetCritical().GetValue()
			}
			if suites[i].build.GetCritical() != suites[j].build.GetCritical() {
				// critical builds at the front
				return suites[i].build.GetCritical() == bbproto.Trinary_YES
			}
			if suites[i].isVm != suites[j].isVm {
				// always prefer VM tests
				return suites[i].isVm
			}
			if !suites[i].isVm && !suites[j].isVm {
				// then prefer the board with the least oversubscription
				if boardPriorities[suites[i].skylabBoard] != boardPriorities[suites[j].skylabBoard] {
					return boardPriorities[suites[i].skylabBoard] < boardPriorities[suites[j].skylabBoard]
				}
			}
			// finally sort by name, just for a stable sort
			return suites[i].tsc.GetDisplayName() < suites[j].tsc.GetDisplayName()
		})
	}
	return m
}
