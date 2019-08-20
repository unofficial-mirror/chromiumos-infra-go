// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package chromeos_version

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/chromiumos/infra/go/internal/git"
	"go.chromium.org/luci/common/errors"
)

type VersionComponent string

const (
	Unspecified  VersionComponent = "UNSPECIFIED"
	ChromeBranch VersionComponent = "CHROME_BRANCH"
	Build        VersionComponent = "CHROMEOS_BUILD"
	Branch       VersionComponent = "CHROMEOS_BRANCH"
	Patch        VersionComponent = "CHROMEOS_PATCH"
)

// This is a var and not a const for testing purposes.
var (
	VersionFileProjectPath string = "chromeos/config/chromeos_version.sh"
)

const (
	keyValueRegex string = `(?P<prefix>%s=)(\d+)(?P<suffix>\b)`
	pushBranch    string = "tmp_checkin_branch"
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

// VersionsEqual returns true if the two versions are equal, and false otherwise.
func VersionsEqual(a, b VersionInfo) bool {
	return (a.ChromeBranch == b.ChromeBranch &&
		a.BuildNumber == b.BuildNumber &&
		a.BranchBuildNumber == b.BranchBuildNumber &&
		a.PatchNumber == b.PatchNumber)
}

// GetVersionInfoFromRepo reads version info from a fixed location in the specified repository.
func GetVersionInfoFromRepo(sourceRepo string) (VersionInfo, error) {
	versionFile := filepath.Join(sourceRepo, VersionFileProjectPath)

	fileData, err := ioutil.ReadFile(versionFile)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("could not read version file %s", versionFile)
	}

	v, err := ParseVersionInfo(fileData)
	v.VersionFile = versionFile
	return v, err
}

// ParseVersionInfo parses file contents for version info.
func ParseVersionInfo(fileData []byte) (VersionInfo, error) {
	var v VersionInfo
	fieldsFound := make(map[VersionComponent]bool)
	for field, pattern := range chromeosVersionMapping {
		if match := findValue(pattern, string(fileData)); match != "" {
			num, err := strconv.Atoi(match)
			if err != nil {
				// log.Fatal here because the regex only matches on integers -- there's no way
				// this should be able to happen.
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
			fieldsFound[field] = true
			continue
		}
	}
	for _, field := range []VersionComponent{ChromeBranch, Build, Branch, Patch} {
		_, ok := fieldsFound[field]
		if !ok {
			return v, fmt.Errorf("did not find field %s", string(field))
		}
	}
	return v, nil
}

func findValue(re *regexp.Regexp, line string) string {
	match := re.FindSubmatch([]byte(line))
	if len(match) == 0 {
		return ""
	}
	// Return second submatch (the value).
	return string(match[2])
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

// StrippedVersionString returns the stripped version string of the given
// VersionInfo struct, i.e. the non-zero components of the version.
// Example: 123.1.0 --> 123.1
// Example: 123.0.0 --> 123
func (v *VersionInfo) StrippedVersionString() string {
	var nonzeroVersionComponents []string
	for _, component := range v.VersionComponents() {
		if component == 0 {
			continue
		}
		nonzeroVersionComponents = append(nonzeroVersionComponents, strconv.Itoa(component))
	}
	return strings.Join(nonzeroVersionComponents, `.`)
}

// UpdateVersionFile updates the version file with our current version.
func (v *VersionInfo) UpdateVersionFile() error {
	if v.VersionFile == "" {
		return fmt.Errorf("cannot call UpdateVersionFile without an associated version file (field VersionFile)")
	}

	data, err := ioutil.ReadFile(v.VersionFile)
	if err != nil {
		return fmt.Errorf("could not read version file %s", v.VersionFile)
	}

	fileData := string(data)
	for field, pattern := range chromeosVersionMapping {
		var fieldVal int
		switch field {
		case ChromeBranch:
			fieldVal = v.ChromeBranch
		case Build:
			fieldVal = v.BuildNumber
		case Branch:
			fieldVal = v.BranchBuildNumber
		case Patch:
			fieldVal = v.PatchNumber
		default:
			// This should never happen.
			log.Fatal("Invalid version component.")
		}

		// Update version component value in file contents.
		newVersionTemplate := fmt.Sprintf("${prefix}%d${suffix}", fieldVal)
		fileData = pattern.ReplaceAllString(fileData, newVersionTemplate)
	}

	repoDir := filepath.Dir(v.VersionFile)
	// Create new branch.
	if err = git.CreateBranch(repoDir, pushBranch); err != nil {
		return err
	}
	// Update version file.
	if err = ioutil.WriteFile(v.VersionFile, []byte(fileData), 0644); err != nil {
		return errors.Annotate(err, "could not write version file %s", v.VersionFile).Err()
	}

	return nil
}