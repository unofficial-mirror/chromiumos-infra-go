// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package pointless

import (
	"github.com/bmatcuk/doublestar"
	chromite "go.chromium.org/chromiumos/infra/proto/go/chromite/api"
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
	"testing"
	"testplans/internal/git"
)

func makeBuildbucketBuild(changes []*bbproto.GerritChange) *bbproto.Build {
	b := &bbproto.Build{
		Input:   &bbproto.Build_Input{},
		Builder: &bbproto.BuilderID{Builder: "reef"},
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
			Files:   []string{"relevantfile", "irrelevantdir2"},
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
	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantSourcePaths: []*testplans_pb.SourceTree{
			{Path: "src/pub/ex/irrelevantdir"},
		},
	}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, cfg)
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
	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantSourcePaths: []*testplans_pb.SourceTree{
			{Path: "src/internal/catpics"},
		},
	}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, cfg)
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
			Files: []string{
				"chromite-maybe/config-thing/file1",
				"chromite-maybe/other-config",
				"chromite-maybe/somedir/img_123.jpg",
				"chromite-maybe/someotherdir/ignore_me.txt",
			},
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

	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantFilePatterns: []*testplans_pb.FilePattern{
			{Pattern: "**/ignore_me.txt"},
		},
		IrrelevantSourcePaths: []*testplans_pb.SourceTree{
			{Path: "src/pub/ex/chromite-maybe/config-thing"},
			{Path: "src/pub/ex/chromite-maybe/other-config"},
		},
		IrrelevantFileBaseNames: []*testplans_pb.FileBaseName{
			{Name: "img_123.jpg"},
		},
	}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, cfg)
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
	cfg := testplans_pb.BuildIrrelevanceCfg{}

	res, err := CheckBuilder(build, chRevData, depGraph, repoToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if res.BuildIsPointless.Value {
		t.Errorf("expected !build_is_pointless, instead got result %v", res)
	}
}

func TestCheckBuild_nilDepGraphSuccessWithNoFilter(t *testing.T) {
	// In this test, no DepGraph is provided. We expect the checker to finish successfully, and to not
	// filter out the files.

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
	repoToSrcRoot := map[string]string{
		"chromiumos/public/example": "src/pub/ex",
	}
	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantSourcePaths: []*testplans_pb.SourceTree{
			{Path: "src/internal/catpics"},
		},
	}

	res, err := CheckBuilder(build, chRevData, nil, repoToSrcRoot, cfg)
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
