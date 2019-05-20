// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package git

import (
	"context"
	"encoding/base64"
	"github.com/golang/mock/gomock"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"net/http"
	"testing"
)

func TestFetchFilesFromGitiles_success(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	// This is a base64-encoded .tar.gz file. It contains one file, contents pair:
	// dir/file1, This is a gzipped file!
	base64Enc := `H4sIADj/sFwAA+2VQU7DMBBFs+4phgu0HntmTBfds+yCCxjFtJYIjZxGKpweh6pBhYJYYBCtnyxF
iiP5O877WbrdjXe1j7M6xCoPSinLDJV9BSrNlrWB4f4eRkBDTGhFUIFCEdYV7DLlOaLvti6mKJ13
j+4uurVrTj7XxtC4+LSKm749Nb/fCYzXfwIS9KFeaJkrvp5oC802NH6BzIzKIOqpzA1pPfnroIUs
JOtnudc4+I8KmZT+6H/y5dh/spT859zBBi7c/+H8l+M/4D48ePzxNdL7EKJv9b8loXT+VtiW/v8N
vu7/pOsUtTZkSv+fJ4P/eax/4+D/p/1v1Dv/WUQqUBkzjVy4/7fr0EEaDlbPoW19DcPXcFV0LxQK
hTPnBcGXkjUAEgAA
`
	encodedZip, err := base64.StdEncoding.DecodeString(base64Enc)
	if err != nil {
		t.Error(err)
	}

	gitilesMock := gitilespb.NewMockGitilesClient(ctl)
	gitilesMock.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(
		&gitilespb.ArchiveResponse{
			Contents: encodedZip,
		},
		nil,
	)
	host := "limited-review.googlesource.com"
	project := "chromiumos/for/the/win"
	ref := "master"
	paths := []string{"dir/file1"}
	MockGitiles = gitilesMock

	m, err := FetchFilesFromGitiles(http.DefaultClient, context.Background(), host, project, ref, paths)
	if err != nil {
		t.Error(err)
	}

	v, found := (*m)["dir/file1"]
	if !found {
		t.Error("Expected file not found in archive")
	}
	if v != "This is a gzipped file!\n" {
		t.Error("Archive not unzipped correctly")
	}
}
