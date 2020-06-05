// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package build_plan

import (
	"testing"

	"go.chromium.org/chromiumos/infra/go/internal/gerrit"
	cros_pb "go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	testplans_pb "go.chromium.org/chromiumos/infra/proto/go/testplans"
	bbproto "go.chromium.org/luci/buildbucket/proto"
)

func makeBuilderConfig(name string, isImageBuilder bool, rwMode cros_pb.BuilderConfig_General_RunWhen_Mode, rwPatterns []string) *cros_pb.BuilderConfig {
	b := &cros_pb.BuilderConfig{
		Id: &cros_pb.BuilderConfig_Id{
			Name: name,
		},
		General: &cros_pb.BuilderConfig_General{
			RunWhen: &cros_pb.BuilderConfig_General_RunWhen{
				Mode:         rwMode,
				FilePatterns: rwPatterns,
			},
		},
		Artifacts: &cros_pb.BuilderConfig_Artifacts{},
	}
	if isImageBuilder {
		b.Artifacts.ArtifactTypes = append(b.GetArtifacts().GetArtifactTypes(), cros_pb.BuilderConfig_Artifacts_IMAGE_ZIP)
	}
	return b
}

func TestCheckBuilders_imageBuilderFiltering(t *testing.T) {
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
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example": {"refs/heads/master": "src/pub/ex"},
	}

	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantFilePatterns: []*testplans_pb.FilePattern{
			{Pattern: "**/ignore_me.txt"},
		},
	}

	b := []*cros_pb.BuilderConfig{
		makeBuilderConfig("my_image_builder", true, cros_pb.BuilderConfig_General_RunWhen_ALWAYS_RUN, []string{}),
		makeBuilderConfig("not_an_image_builder", false, cros_pb.BuilderConfig_General_RunWhen_ALWAYS_RUN, []string{}),
	}

	res, err := CheckBuilders(b, changes, chRevData, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if len(res.BuildsToRun) != 1 {
		t.Errorf("Expected BuildsToRun to have 1 element. Instead, %v", res.BuildsToRun)
	}
	if res.BuildsToRun[0].GetName() != "not_an_image_builder" {
		t.Errorf("Expected res.BuildsToRun[0].GetName() == \"not_an_image_builder\". Instead, %v", res.BuildsToRun[0].GetName())
	}
	if len(res.SkipForRunWhenRules) != 0 {
		t.Errorf("Expected SkipForRunWhenRules to be empty. Instead, %v", res.SkipForRunWhenRules)
	}
	if len(res.SkipForGlobalBuildIrrelevance) != 1 {
		t.Errorf("Expected SkipForGlobalBuildIrrelevance to have 1 element. Instead, %v", res.SkipForGlobalBuildIrrelevance)
	}
	if res.SkipForGlobalBuildIrrelevance[0].GetName() != "my_image_builder" {
		t.Errorf("Expected SkipForGlobalBuildIrrelevance[0].GetName() == \"my_image_builder\", instead %v", res.SkipForGlobalBuildIrrelevance[0].GetName())
	}
}

func TestCheckBuilders_noGerritChanges(t *testing.T) {
	// When there are no GerritChanges, we run all of the builders.

	changes := []*bbproto.GerritChange{}
	chRevData := gerrit.GetChangeRevsForTest([]*gerrit.ChangeRev{})
	repoToBranchToSrcRoot := map[string]map[string]string{}

	cfg := testplans_pb.BuildIrrelevanceCfg{
		IrrelevantFilePatterns: []*testplans_pb.FilePattern{
			{Pattern: "**/ignore_me.txt"},
		},
	}

	b := []*cros_pb.BuilderConfig{
		makeBuilderConfig("my_image_builder", true, cros_pb.BuilderConfig_General_RunWhen_ALWAYS_RUN, []string{}),
		makeBuilderConfig("not_an_image_builder", false, cros_pb.BuilderConfig_General_RunWhen_ALWAYS_RUN, []string{}),
		makeBuilderConfig("only_run_on_match", true, cros_pb.BuilderConfig_General_RunWhen_ONLY_RUN_ON_FILE_MATCH, []string{"**/match_me.txt"}),
		makeBuilderConfig("no_run_on_match", true, cros_pb.BuilderConfig_General_RunWhen_NO_RUN_ON_FILE_MATCH, []string{"not/a/real/dir"}),
	}

	res, err := CheckBuilders(b, changes, chRevData, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if len(res.BuildsToRun) != 4 {
		t.Errorf("Expected BuildsToRun to have 4 elements. Instead, %v", res.BuildsToRun)
	}
	if len(res.SkipForRunWhenRules) != 0 {
		t.Errorf("Expected SkipForRunWhenRules to be empty. Instead, %v", res.SkipForRunWhenRules)
	}
	if len(res.SkipForGlobalBuildIrrelevance) != 0 {
		t.Errorf("Expected SkipForGlobalBuildIrrelevance to be empty. Instead, %v", res.SkipForGlobalBuildIrrelevance)
	}
}

func TestCheckBuilders_onlyRunOnFileMatch(t *testing.T) {
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
				"chromite-maybe/someotherdir/match_me.txt",
			},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example": {"refs/heads/master": "src/pub/ex"},
	}

	cfg := testplans_pb.BuildIrrelevanceCfg{}

	b := []*cros_pb.BuilderConfig{
		makeBuilderConfig("board_to_run", true, cros_pb.BuilderConfig_General_RunWhen_ONLY_RUN_ON_FILE_MATCH, []string{"**/match_me.txt"}),
		makeBuilderConfig("board_to_skip", true, cros_pb.BuilderConfig_General_RunWhen_ONLY_RUN_ON_FILE_MATCH, []string{"not/a/real/dir"}),
	}

	res, err := CheckBuilders(b, changes, chRevData, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if len(res.BuildsToRun) != 1 {
		t.Errorf("Expected BuildsToRun to have 1 element. Instead, %v", res.BuildsToRun)
	}
	if res.BuildsToRun[0].GetName() != "board_to_run" {
		t.Errorf("Expected res.BuildsToRun[0].GetName() == \"board_to_run\". Instead, %v", res.BuildsToRun[0].GetName())
	}
	if len(res.SkipForGlobalBuildIrrelevance) != 0 {
		t.Errorf("Expected SkipForGlobalBuildIrrelevance to be empty. Instead, %v", res.SkipForGlobalBuildIrrelevance)
	}
	if len(res.SkipForRunWhenRules) != 1 {
		t.Errorf("Expected SkipForRunWhenRules to have 1 element. Instead, %v", res.SkipForRunWhenRules)
	}
	if res.SkipForRunWhenRules[0].GetName() != "board_to_skip" {
		t.Errorf("Expected SkipForRunWhenRules[0].GetName() == \"board_to_skip\", instead %v", res.SkipForRunWhenRules[0].GetName())
	}
}

func TestCheckBuilders_NoRunOnFileMatch(t *testing.T) {
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
				"chromite-maybe/somedir/match_me_1.txt",
				"chromite-maybe/someotherdir/match_me_2.txt",
			},
		},
	})
	repoToBranchToSrcRoot := map[string]map[string]string{
		"chromiumos/public/example": {"refs/heads/master": "src/pub/ex"},
	}

	cfg := testplans_pb.BuildIrrelevanceCfg{}

	b := []*cros_pb.BuilderConfig{
		makeBuilderConfig("board_to_skip", true, cros_pb.BuilderConfig_General_RunWhen_NO_RUN_ON_FILE_MATCH, []string{"**/match_me_1.txt", "**/match_me_2.txt"}),
		makeBuilderConfig("board_to_run", true, cros_pb.BuilderConfig_General_RunWhen_NO_RUN_ON_FILE_MATCH, []string{"not/a/real/dir"}),
	}

	res, err := CheckBuilders(b, changes, chRevData, repoToBranchToSrcRoot, cfg)
	if err != nil {
		t.Error(err)
	}
	if len(res.BuildsToRun) != 1 {
		t.Errorf("Expected BuildsToRun to have 1 element. Instead, %v", res.BuildsToRun)
	}
	if res.BuildsToRun[0].GetName() != "board_to_run" {
		t.Errorf("Expected res.BuildsToRun[0].GetName() == \"board_to_run\". Instead, %v", res.BuildsToRun[0].GetName())
	}
	if len(res.SkipForGlobalBuildIrrelevance) != 0 {
		t.Errorf("Expected SkipForGlobalBuildIrrelevance to be empty. Instead, %v", res.SkipForGlobalBuildIrrelevance)
	}
	if len(res.SkipForRunWhenRules) != 1 {
		t.Errorf("Expected SkipForRunWhenRules to have 1 element. Instead, %v", res.SkipForRunWhenRules)
	}
	if res.SkipForRunWhenRules[0].GetName() != "board_to_skip" {
		t.Errorf("Expected SkipForRunWhenRules[0].GetName() == \"board_to_skip\", instead %v", res.SkipForRunWhenRules[0].GetName())
	}
}
