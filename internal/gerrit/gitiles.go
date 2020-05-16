// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package gerrit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"time"

	"go.chromium.org/chromiumos/infra/go/internal/shared"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
)

var (
	// Override this to use a mock GitilesClient rather than the real one.
	MockGitiles gitilespb.GitilesClient
)

// FetchFilesFromGitiles fetches file contents from gitiles.
//
// project is the git project to fetch from.
// ref is the git-ref to fetch from.
// paths lists the paths inside the git project to fetch contents for.
//
// fetchFilesFromGitiles returns a map from path in the git project to the
// contents of the file at that path for each requested path.
func FetchFilesFromGitiles(authedClient *http.Client, ctx context.Context, host, project, ref string, paths []string) (*map[string]string, error) {
	var gc gitilespb.GitilesClient
	var err error
	if MockGitiles != nil {
		gc = MockGitiles
	} else {
		if gc, err = gitiles.NewRESTClient(authedClient, host, true); err != nil {
			return nil, err
		}
	}
	contents, err := obtainGitilesBytes(ctx, gc, project, ref)
	if err != nil {
		return nil, err
	}
	return extractGitilesArchive(ctx, contents, paths)
}

func obtainGitilesBytes(ctx context.Context, gc gitilespb.GitilesClient, project string, ref string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	ch := make(chan *gitilespb.ArchiveResponse, 1)

	err := shared.DoWithRetry(ctx, shared.LongerOpts, func() error {
		// This sets the deadline for the individual API call, while the outer context sets
		// an overall timeout for all attempts.
		innerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		req := &gitilespb.ArchiveRequest{
			Project: project,
			Ref:     ref,
			Format:  gitilespb.ArchiveRequest_GZIP,
		}
		a, err := gc.Archive(innerCtx, req)
		if err != nil {
			return errors.Annotate(err, "obtain gitiles archive").Err()
		}
		logging.Debugf(ctx, "Gitiles archive %+v size: %d", req, len(a.Contents))
		ch <- a
		return nil
	})
	if err != nil {
		return nil, err
	}
	a := <-ch
	return a.Contents, nil
}

// extractGitilesArchive extracts file at each path in paths from the given
// gunzipped tarfile.
//
// extractGitilesArchive returns a map from path to the content of the file at
// that path in the archives for each requested path found in the archive.
//
// This function takes ownership of data. Caller should not use the byte array
// concurrent to / after this call. See io.Reader interface for more details.
func extractGitilesArchive(ctx context.Context, data []byte, paths []string) (*map[string]string, error) {
	pmap := make(map[string]bool)
	for _, p := range paths {
		pmap[p] = true
	}

	abuf := bytes.NewBuffer(data)
	gr, err := gzip.NewReader(abuf)
	if err != nil {
		return nil, errors.Annotate(err, "extract gitiles archive").Err()
	}
	defer gr.Close()

	res := make(map[string]string)
	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		switch {
		case err == io.EOF:
			// Scanned all files.
			return &res, nil
		case err != nil:
			return nil, errors.Annotate(err, "extract gitiles archive").Err()
		default:
			// good case.
		}
		if found := pmap[h.Name]; !found {
			continue
		}

		logging.Debugf(ctx, "Inventory data file %s size %d", h.Name, h.Size)
		data := make([]byte, h.Size)
		if _, err := io.ReadFull(tr, data); err != nil {
			return nil, errors.Annotate(err, "extract gitiles archive").Err()
		}
		res[h.Name] = string(data)
	}
}
