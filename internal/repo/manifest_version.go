// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package repo

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
)

type VersionComponent string

const (
	ChromeBranch VersionComponent = "CHROME_BRANCH"
	Build        VersionComponent = "CHROMEOS_BUILD"
	Branch       VersionComponent = "CHROMEOS_BRANCH"
	Patch        VersionComponent = "CHROMEOS_PATCH"
)

// This is a var and not a const for testing purposes.
var (
	versionFilePath string = "src/third_party/chromiumos-overlay/" +
		"chromeos/config/chromeos_version.sh"
)

const (
	keyValueRegex string = `%s=(\d+)\b`
)

var chromeosVersionMapping = map[VersionComponent](*regexp.Regexp){
	ChromeBranch: regexp.MustCompile(fmt.Sprintf(keyValueRegex, ChromeBranch)),
	Build:        regexp.MustCompile(fmt.Sprintf(keyValueRegex, Build)),
	Branch:       regexp.MustCompile(fmt.Sprintf(keyValueRegex, Branch)),
	Patch:        regexp.MustCompile(fmt.Sprintf(keyValueRegex, Patch)),
}

type VersionInfo struct {
	ChromeBranch      int
	BuildNumber       int
	BranchBuildNumber int
	PatchNumber       int
	VersionFile       string
}

func GetVersionInfoFromRepo(sourceRepo string) (VersionInfo, error) {
	var v VersionInfo
	v.VersionFile = filepath.Join(sourceRepo, versionFilePath)

	fileData, err := ioutil.ReadFile(v.VersionFile)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("could not read version file %s", v.VersionFile)
	}

	for field, pattern := range chromeosVersionMapping {
		if match := findValue(pattern, string(fileData)); match != "" {
			num, err := strconv.Atoi(match)
			if err != nil {
				log.Fatal(fmt.Sprintf("%s value %s could not be converted to integer.", field, match))
			}
			switch field {
			case ChromeBranch:
				v.ChromeBranch = num
			case Build:
				v.BuildNumber = num
			case Branch:
				v.BranchBuildNumber = num
			case Patch:
				v.PatchNumber = num
			default:
				// This should never happen.
				log.Fatal("Invalid version component.")
			}
			continue
		}
	}

	return v, nil
}

func findValue(re *regexp.Regexp, line string) string {
	match := re.FindSubmatch([]byte(line))
	if len(match) == 0 {
		return ""
	}
	// Return first submatch (the value).
	return string(match[1])
}

func (v *VersionInfo) IncrementVersion(incrType VersionComponent) string {
	if incrType == ChromeBranch {
		v.ChromeBranch += 1
	}

	// Increment build_number for ChromeBranch incrType to avoid
	// crbug.com/213075.
	if incrType == ChromeBranch || incrType == Build {
		v.BuildNumber += 1
		v.BranchBuildNumber = 0
		v.PatchNumber = 0
	} else if incrType == Branch && v.PatchNumber == 0 {
		v.BranchBuildNumber += 1
	} else {
		v.PatchNumber += 1
	}

	return v.VersionString()
}

func incrString(str string) string {
	num, err := strconv.Atoi(str)
	if err != nil {
		log.Fatal(fmt.Sprintf("String %s could not be converted to integer.", str))
	}
	return strconv.Itoa(num + 1)
}

func (v *VersionInfo) VersionString() string {
	return fmt.Sprintf("%d.%d.%d", v.BuildNumber, v.BranchBuildNumber, v.PatchNumber)
}

func (v *VersionInfo) VersionComponents() []int {
	return []int{v.BuildNumber, v.BranchBuildNumber, v.PatchNumber}
}
