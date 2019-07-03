package repo

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type IncrType string

const (
	ChromeBranch IncrType = "chrome_branch"
	Build        IncrType = "build"
	Branch       IncrType = "branch"
	Patch        IncrType = "patch"
)

// This is a var and not a const for testing purposes.
var (
	versionFilePath string = "src/third_party/chromiumos-overlay/" +
		"chromeos/config/chromeos_version.sh"
)

const (
	keyValueRegex string = `%s=(\d+)\s*$`
)

var (
	chromeBranchRegex   *regexp.Regexp = regexp.MustCompile(fmt.Sprintf(keyValueRegex, "CHROME_BRANCH"))
	chromeosBuildRegex  *regexp.Regexp = regexp.MustCompile(fmt.Sprintf(keyValueRegex, "CHROMEOS_BUILD"))
	chromeosBranchRegex *regexp.Regexp = regexp.MustCompile(fmt.Sprintf(keyValueRegex, "CHROMEOS_BRANCH"))
	chromeosPatchRegex  *regexp.Regexp = regexp.MustCompile(fmt.Sprintf(keyValueRegex, "CHROMEOS_PATCH"))
)

type VersionInfo struct {
	BuildNumber       string
	BranchBuildNumber string
	PatchNumber       string
	ChromeBranch      string
	VersionFile       string
	incrType          IncrType
}

func GetVersionInfoFromRepo(sourceRepo string, incrType IncrType) (VersionInfo, error) {
	var v VersionInfo
	v.VersionFile = filepath.Join(sourceRepo, versionFilePath)

	fileData, err := ioutil.ReadFile(v.VersionFile)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("could not read version file %s", v.VersionFile)
	}

	for _, line := range strings.Split(string(fileData), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if match := findValue(chromeBranchRegex, line); match != "" {
			v.ChromeBranch = match
			log.Printf("Set the Chrome branch number to: %s", v.ChromeBranch)
			continue
		}
		if match := findValue(chromeosBuildRegex, line); match != "" {
			v.BuildNumber = match
			log.Printf("Set the Chrome branch number to: %s", v.BuildNumber)
			continue
		}
		if match := findValue(chromeosBranchRegex, line); match != "" {
			v.BranchBuildNumber = match
			log.Printf("Set the Chrome branch number to: %s", v.BranchBuildNumber)
			continue
		}
		if match := findValue(chromeosPatchRegex, line); match != "" {
			v.PatchNumber = match
			log.Printf("Set the Chrome branch number to: %s", v.PatchNumber)
			continue
		}
	}
	if incrType == "" {
		incrType = Build
	}
	v.incrType = incrType
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

func (v *VersionInfo) IncrementVersion() string {
	if v.incrType == ChromeBranch {
		v.ChromeBranch = incrString(v.ChromeBranch)
	}

	// Increment build_number for ChromeBranch incrType to avoid
	// crbug.com/213075.
	if v.incrType == ChromeBranch || v.incrType == Build {
		v.BuildNumber = incrString(v.BuildNumber)
		v.BranchBuildNumber = "0"
		v.PatchNumber = "0"
	} else if v.incrType == Branch && v.PatchNumber == "0" {
		v.BranchBuildNumber = incrString(v.BranchBuildNumber)
	} else {
		v.PatchNumber = incrString(v.PatchNumber)
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
	return fmt.Sprintf("%s.%s.%s", v.BuildNumber, v.BranchBuildNumber, v.PatchNumber)
}
