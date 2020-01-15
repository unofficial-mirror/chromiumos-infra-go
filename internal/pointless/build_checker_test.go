// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package pointless

import (
	"github.com/bmatcuk/doublestar"
	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	chromite "go.chromium.org/chromiumos/infra/proto/go/chromite/api"
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"testing"
)

func TestCheckBuilder_irrelevantToDepGraph(t *testing.T) {
	// In this test, there's a CL that is fully irrelevant to the dep graph, so the build is pointless.

	changes := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2, Project: "chromiumos/public/example"}}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/public/example",
			Files:   []string{"relevantfile", "irrelevantdir2"},
		},
	})
	depGraph := &chromite.DepGraph{}
	relevantPaths := []*testplans_pb.PointlessBuildCheckRequest_Path{{Path: "src/dep/graph/path"}}
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example": {"refs/heads/master": "src/pub/ex"},
	}
	cfg := testplans_pb.BuildIrrelevanceCfg{}

	res, err := CheckBuilder(changes, chRevData, depGraph, relevantPaths, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if !res.BuildIsPointless.Value {
		t.Errorf("expected build_is_pointless, instead got result %v", res)
	}
	if res.PointlessBuildReason != testplans_pb.PointlessBuildCheckResponse_IRRELEVANT_TO_DEPS_GRAPH {
		t.Errorf("expected IRRELEVANT_TO_DEPS_GRAPH, instead got result %v", res)
	}
}

func TestCheckBuilder_relevantToDepGraph(t *testing.T) {
	// In this test, there are two CLs, with one of them being relevant to the Portage graph. The
	// build thus is necessary.

	changes := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2, Project: "chromiumos/public/example"},
		{Host: "test-internal-review.googlesource.com", Change: 234, Patchset: 3, Project: "chromiumos/internal/example"}}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/public/example",
			Files:   []string{"a/b/c"},
		},
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-internal-review.googlesource.com",
				ChangeNum: 234,
				Revision:  3,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/internal/example",
			Files:   []string{"important_stuff/important_file"},
		},
	})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/internal/ex/important_stuff"},
			}}}}
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example":   {"refs/heads/master": "src/pub/ex"},
		"chromiumos/internal/example": {"refs/heads/master": "src/internal/ex"},
	}
	cfg := testplans_pb.BuildIrrelevanceCfg{}

	res, err := CheckBuilder(changes, chRevData, depGraph, []*testplans_pb.PointlessBuildCheckRequest_Path{}, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if res.BuildIsPointless.Value {
		t.Errorf("expected !build_is_pointless, instead got result %v", res)
	}
}

func TestCheckBuilder_buildIrrelevantPaths(t *testing.T) {
	// In this test, the only files touched are those that are explicitly listed as being not relevant
	// to Portage.

	changes := []*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2, Project: "chromiumos/public/example"}}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{
		{
			ChangeRevKey: gerrit.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Branch:  "refs/heads/master",
			Project: "chromiumos/public/example",
			Files: []string{
				"chromite-maybe/someotherdir/ignore_me.txt",
			},
		},
	})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/pub/ex/chromite-maybe"},
			}}}}
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example": {"refs/heads/master": "src/pub/ex"},
	}

	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantFilePatterns: []*testplans_pb.FilePattern{
			{Pattern: "**/ignore_me.txt"},
		},
	}

	res, err := CheckBuilder(changes, chRevData, depGraph, []*testplans_pb.PointlessBuildCheckRequest_Path{}, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if !res.BuildIsPointless.Value {
		t.Errorf("expected build_is_pointless, instead got result %v", res)
	}
	if res.PointlessBuildReason != testplans_pb.PointlessBuildCheckResponse_IRRELEVANT_TO_KNOWN_NON_PORTAGE_DIRECTORIES {
		t.Errorf("expected IRRELEVANT_TO_KNOWN_NON_PORTAGE_DIRECTORIES, instead got result %v", res)
	}
}

func TestCheckBuilder_noGerritChangesMeansNecessaryBuild(t *testing.T) {
	var changes []*bbproto.GerritChange
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/pub/ex/chromite-maybe"},
			}}}}
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example": {"refs/heads/master": "src/pub/ex"},
	}
	cfg := testplans_pb.BuildIrrelevanceCfg{}

	res, err := CheckBuilder(changes, chRevData, depGraph, []*testplans_pb.PointlessBuildCheckRequest_Path{}, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if res.BuildIsPointless.Value {
		t.Errorf("expected !build_is_pointless, instead got result %v", res)
	}
}

func match(t *testing.T, pattern, name string) {
	m, err := doublestar.Match(pattern, name)
	if err != nil {
		t.Errorf("error trying to match pattern %s against name %s: %v", pattern, name, err)
	} else {
		if !m {
			t.Errorf("expected pattern %s to match against name %s, but it did not match", pattern, name)
		}
	}
}

func notMatch(t *testing.T, pattern, name string) {
	m, err := doublestar.Match(pattern, name)
	if err != nil {
		t.Errorf("error trying to match pattern %s against name %s: %v", pattern, name, err)
	} else {
		if m {
			t.Errorf("expected pattern %s not to match against name %s, but it did match", pattern, name)
		}
	}
}

func TestDoubleStar(t *testing.T) {
	// A test that demonstrates/verifies operation of the doublestar matching package.

	match(t, "**/OWNERS", "OWNERS")
	match(t, "**/OWNERS", "some/deep/subdir/OWNERS")
	notMatch(t, "**/OWNERS", "OWNERS/fds")

	match(t, "chromite/config/**", "chromite/config/config_dump.json")

	match(t, "**/*.md", "a/b/c/README.md")
}
