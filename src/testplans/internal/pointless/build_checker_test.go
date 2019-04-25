// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package pointless

import (
	_struct "github.com/golang/protobuf/ptypes/struct"
	chromite "go.chromium.org/chromiumos/infra/proto/go/chromite/api"
	"go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"testing"
	"testplans/internal/git"
)

func makeBuildbucketBuild(changes []*bbproto.GerritChange) *bbproto.Build {
	b := &bbproto.Build{
		Input: &bbproto.Build_Input{},
		Output: &bbproto.Build_Output{
			Properties: &_struct.Struct{
				Fields: map[string]*_struct.Value{
					"build_target": {
						Kind: &_struct.Value_StructValue{StructValue: &_struct.Struct{
							Fields: map[string]*_struct.Value{
								"name": {Kind: &_struct.Value_StringValue{StringValue: "reef"}},
							},
						}},
					},
				}}},
	}
	for _, c := range changes {
		b.Input.GerritChanges = append(b.Input.GerritChanges, c)
	}
	return b
}

func TestCheckBuilder_irrelevantToDepGraph(t *testing.T) {
	// In this test, there's a CL that is fully irrelevant to the dep graph, so the build is pointless.

	build := makeBuildbucketBuild([]*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2, Project: "chromiumos/public/example"}})
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Project: "chromiumos/public/example",
			Files:   []string{"a/b/c"},
		},
	})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/dep/graph/path"},
			}}}}
	repoToSrcRoot := map[string]string{
		"chromiumos/public/example": "src/pub/ex",
	}
	buildIrrelevantPaths := []string{"src/internal/catpics"}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, buildIrrelevantPaths)
	if err != nil {
		t.Error(err)
	}
	if !res.BuildIsPointless.Value {
		t.Errorf("expected build_is_pointless, instead got result %v", res)
	}
	if res.PointlessBuildReason != chromiumos.PointlessBuildCheckResponse_IRRELEVANT_TO_DEPS_GRAPH {
		t.Errorf("expected IRRELEVANT_TO_DEPS_GRAPH, instead got result %v", res)
	}
}

func TestCheckBuilder_relevantToDepGraph(t *testing.T) {
	// In this test, there are two CLs, with one of them being relevant to the Portage graph. The
	// build thus is necessary.

	build := makeBuildbucketBuild([]*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2, Project: "chromiumos/public/example"},
		{Host: "test-internal-review.googlesource.com", Change: 234, Patchset: 3, Project: "chromiumos/internal/example"}})
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Project: "chromiumos/public/example",
			Files:   []string{"a/b/c"},
		},
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-internal-review.googlesource.com",
				ChangeNum: 234,
				Revision:  3,
			},
			Project: "chromiumos/internal/example",
			Files:   []string{"important_stuff/important_file"},
		},
	})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/internal/ex/important_stuff"},
			}}}}
	repoToSrcRoot := map[string]string{
		"chromiumos/public/example":   "src/pub/ex",
		"chromiumos/internal/example": "src/internal/ex",
	}
	buildIrrelevantPaths := []string{"src/internal/catpics"}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, buildIrrelevantPaths)
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

	build := makeBuildbucketBuild([]*bbproto.GerritChange{
		{Host: "test-review.googlesource.com", Change: 123, Patchset: 2, Project: "chromiumos/public/example"}})
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{
		{
			ChangeRevKey: git.ChangeRevKey{
				Host:      "test-review.googlesource.com",
				ChangeNum: 123,
				Revision:  2,
			},
			Project: "chromiumos/public/example",
			Files:   []string{"chromite-maybe/config-thing/file1", "chromite-maybe/other-config/file2"},
		},
	})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/pub/ex/chromite-maybe"},
			}}}}
	repoToSrcRoot := map[string]string{
		"chromiumos/public/example": "src/pub/ex",
	}
	buildIrrelevantPaths := []string{"src/pub/ex/chromite-maybe/config-thing", "src/pub/ex/chromite-maybe/other-config"}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, buildIrrelevantPaths)
	if err != nil {
		t.Error(err)
	}
	if !res.BuildIsPointless.Value {
		t.Errorf("expected build_is_pointless, instead got result %v", res)
	}
	if res.PointlessBuildReason != chromiumos.PointlessBuildCheckResponse_IRRELEVANT_TO_KNOWN_NON_PORTAGE_DIRECTORIES {
		t.Errorf("expected IRRELEVANT_TO_KNOWN_NON_PORTAGE_DIRECTORIES, instead got result %v", res)
	}
}

func TestCheckBuilder_noGerritChangesMeansNecessaryBuild(t *testing.T) {
	build := makeBuildbucketBuild([]*bbproto.GerritChange{})
	chRevData := git.GetChangeRevsForTest([]*git.ChangeRev{})
	depGraph := &chromite.DepGraph{
		PackageDeps: []*chromite.PackageDepInfo{
			{DependencySourcePaths: []*chromite.SourcePath{
				{Path: "src/pub/ex/chromite-maybe"},
			}}}}
	repoToSrcRoot := map[string]string{
		"chromiumos/public/example": "src/pub/ex",
	}
	buildIrrelevantPaths := []string{""}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, buildIrrelevantPaths)
	if err != nil {
		t.Error(err)
	}
	if res.BuildIsPointless.Value {
		t.Errorf("expected !build_is_pointless, instead got result %v", res)
	}
}
