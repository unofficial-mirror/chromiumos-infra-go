// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"strings"
)

var (
	configDumpJsonPath = flag.String("config_dump_json_path", "", "Path to fully expanded config_dump.json")
)

// A renamed []string for the purpose of having a custom String() method.
type SSlice []string

func (strings SSlice) String() string {
	str := "["
	for _, s := range strings {
		str += fmt.Sprintf("\n    %s,", s)
	}
	str += "\n  ]"
	return str
}

type TestSuites struct {
	gceTestSuites    []string
	hwTestSuites     []string
	moblabTestSuites []string
	tastVmTestSuites []string
	vmTestSuites     []string
}

func mergeDedupeSortSlice(s1 []string, s2 []string) []string {
	if s1 == nil && s2 == nil {
		return nil
	}
	allStrings := make(map[string]bool)
	for _, s := range s1 {
		allStrings[s] = true
	}
	for _, s := range s2 {
		allStrings[s] = true
	}
	result := make([]string, 0)
	for k := range allStrings {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func (ts1 *TestSuites) merge(ts2 *TestSuites) {
	ts1.gceTestSuites = mergeDedupeSortSlice(ts1.gceTestSuites, ts2.gceTestSuites)
	ts1.hwTestSuites = mergeDedupeSortSlice(ts1.hwTestSuites, ts2.hwTestSuites)
	ts1.moblabTestSuites = mergeDedupeSortSlice(ts1.moblabTestSuites, ts2.moblabTestSuites)
	ts1.tastVmTestSuites = mergeDedupeSortSlice(ts1.tastVmTestSuites, ts2.tastVmTestSuites)
	ts1.vmTestSuites = mergeDedupeSortSlice(ts1.vmTestSuites, ts2.vmTestSuites)
}

func (ts TestSuites) notEmpty() bool {
	return ts.gceTestSuites != nil ||
		ts.hwTestSuites != nil ||
		ts.moblabTestSuites != nil ||
		ts.tastVmTestSuites != nil ||
		ts.vmTestSuites != nil
}

func (ts TestSuites) String() string {
	str := ""
	if ts.gceTestSuites != nil {
		str += fmt.Sprintf("  gce_tests: %v\n", SSlice(ts.gceTestSuites))
	}
	if ts.hwTestSuites != nil {
		str += fmt.Sprintf("  hw_tests: %v\n", SSlice(ts.hwTestSuites))
	}
	if ts.moblabTestSuites != nil {
		str += fmt.Sprintf("  moblab_tests: %v\n", SSlice(ts.moblabTestSuites))
	}
	if ts.tastVmTestSuites != nil {
		str += fmt.Sprintf("  tast_vm_tests: %v\n", SSlice(ts.tastVmTestSuites))
	}
	if ts.vmTestSuites != nil {
		str += fmt.Sprintf("  vm_tests: %v\n", SSlice(ts.vmTestSuites))
	}
	return str
}

func print(suitesByBuilder map[string]TestSuites) {
	builderNames := make([]string, 0)
	for builderName := range suitesByBuilder {
		builderNames = append(builderNames, builderName)
	}
	sort.Strings(builderNames)

	for _, v := range builderNames {
		if suitesByBuilder[v].notEmpty() {
			fmt.Printf("%s\n%v\n", v, suitesByBuilder[v])
		}
	}
}

func main() {
	flag.Parse()
	// Read the SourceTreeConfig JSON file into a proto.
	configDumpJsonBytes, err := ioutil.ReadFile(*configDumpJsonPath)
	if err != nil {
		log.Fatalf("Failed reading config_dump_json_path\n%v", err)
	}

	var topLevelDat map[string]interface{}
	if err = json.Unmarshal(configDumpJsonBytes, &topLevelDat); err != nil {
		log.Fatal(err)
	}
	testSuitesByBuilder := make(map[string]TestSuites)

	for builderName, dataForBuilder := range topLevelDat {
		if strings.HasSuffix(builderName, "-paladin") {
			builderNameWithoutSuffix := strings.TrimSuffix(builderName, "-paladin")
			testSuites := &TestSuites{}
			builderValues := dataForBuilder.(map[string]interface{})
			for fieldKey, fieldValue := range builderValues {
				switch fieldKey {
				case "hw_tests":
					tests := fieldValue.([]interface{})
					for _, testJson := range tests {
						var testDat map[string]interface{}
						if err = json.Unmarshal([]byte(testJson.(string)), &testDat); err != nil {
							log.Fatal(err)
						}
						if testDat != nil && testDat["suite"] != "provision" {
							testSuites.hwTestSuites = append(testSuites.hwTestSuites, testDat["suite"].(string))
						}
					}
				case "vm_tests":
					tests := fieldValue.([]interface{})
					for _, testJson := range tests {
						var testDat map[string]interface{}
						if err = json.Unmarshal([]byte(testJson.(string)), &testDat); err != nil {
							log.Fatal(err)
						}
						if testDat != nil {
							testSuites.vmTestSuites = append(testSuites.vmTestSuites, testDat["test_suite"].(string))
						}
					}
				case "gce_tests":
					tests := fieldValue.([]interface{})
					for _, testJson := range tests {
						var testDat map[string]interface{}
						if err = json.Unmarshal([]byte(testJson.(string)), &testDat); err != nil {
							log.Fatal(err)
						}
						if testDat != nil {
							testSuites.gceTestSuites = append(testSuites.gceTestSuites, testDat["test_suite"].(string))
						}
					}
				case "moblab_vm_tests":
					tests := fieldValue.([]interface{})
					for _, testJson := range tests {

						var testDat map[string]interface{}
						if err = json.Unmarshal([]byte(testJson.(string)), &testDat); err != nil {
							log.Fatal(err)
						}
						if testDat != nil {
							testSuites.moblabTestSuites = append(testSuites.moblabTestSuites, testDat["test_type"].(string))
						}
					}
				case "tast_vm_tests":
					tests := fieldValue.([]interface{})
					for _, testJson := range tests {

						var testDat map[string]interface{}
						if err = json.Unmarshal([]byte(testJson.(string)), &testDat); err != nil {
							log.Fatal(err)
						}
						if testDat != nil {
							testSuites.tastVmTestSuites = append(testSuites.tastVmTestSuites, testDat["suite_name"].(string))
						}
					}
				default:
					// Do nothing
				}
			}
			testSuitesByBuilder[builderNameWithoutSuffix] = *testSuites
		}
	}

	log.Print("\n\n\n")
	log.Printf("Test suites by builder:")

	print(testSuitesByBuilder)
}
