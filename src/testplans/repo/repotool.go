// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"strings"
)

// GetRepoToSourceRoot gets the mapping of Gerrit project to Chromium OS source tree path.
// TODO(seanabraham@chromium.org): Write tests.
func GetRepoToSourceRoot(chromiumosCheckout string) map[string]string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("could not get working dir, %s", err)
	}
	if err := os.Chdir(chromiumosCheckout); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			log.Fatalf("could not change working dir, %s", err)
		}
	}()

	repoToSrcRoot := make(map[string]string)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.Command("repo", "list")
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed trying to run `repo list`: %v\nstderr: %v", err, stderrBuf.String())
	}
	repos := strings.Split(stdoutBuf.String(), "\n")
repoLoop:
	for _, r := range repos {
		if r == "" {
			break repoLoop
		}
		split := strings.Split(r, ":")
		repoName := strings.TrimSpace(split[1])
		srcRoot := strings.TrimSpace(split[0])
		repoToSrcRoot[repoName] = srcRoot
	}
	return repoToSrcRoot
}
