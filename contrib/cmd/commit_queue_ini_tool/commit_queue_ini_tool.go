// Copyright 2019 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"flag"
	"fmt"
	"github.com/mvo5/goconfigparser"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	rootDir = flag.String("root-dir", "", "Root directory to scan. This should be your chromiumos repo checkout path.")
)

// mapWrap is a map with a multiline String method.
type mapWrap map[string]int

func (m mapWrap) String() string {
	str := ""
	for k, v := range m {
		str += fmt.Sprintf("%s: %d\n", k, v)
	}
	return str
}

// getConfigs finds all of the COMMIT-QUEUE.ini files and returns a map of their file paths to their
// contents.
func getConfigs() map[string]*goconfigparser.ConfigParser {
	// Validate --root-dir
	testDir, err := os.Stat(path.Join(*rootDir, "chromite"))
	if err != nil || !testDir.IsDir() {
		log.Fatal("Expected to find a chromite subdirectory of --root-dir. Are you sure that " +
			"--root-dir points to a chromiumos repo checkout?")
	}
	values := make(map[string]*goconfigparser.ConfigParser)
	err = filepath.Walk(*rootDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if f.IsDir() && f.Name() == "chroot" {
			return filepath.SkipDir
		}
		if f.Name() != "COMMIT-QUEUE.ini" {
			return nil
		}
		relPath := strings.TrimPrefix(strings.TrimPrefix(path, *rootDir), "/")
		fmt.Println(relPath)
		cfg := goconfigparser.New()
		if err := cfg.ReadFile(path); err != nil {
			return err
		}
		values[relPath] = cfg
		for _, sec := range cfg.Sections() {
			options, err := cfg.Options(sec)
			if err != nil {
				return err
			}
			for _, opt := range options {
				val, err := cfg.Get(sec, opt)
				if err != nil {
					return err
				}
				fmt.Printf("%s: %s: %s\n", sec, opt, val)
			}
		}
		fmt.Println()
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return values
}

func main() {
	flag.Parse()
	cfgs := getConfigs()
	optionCounts := make(map[string]int)
	valueCounts := make(map[string]int)
	for _, cfg := range cfgs {
		for _, sec := range cfg.Sections() {
			opts, err := cfg.Options(sec)
			if err != nil {
				log.Fatal(err)
			}
			for _, opt := range opts {
				optionCounts[fmt.Sprintf("%s: %s", sec, opt)]++
				val, err := cfg.Get(sec, opt)
				if err != nil {
					log.Fatal(err)
				}
				valueCounts[fmt.Sprintf("%s: %s: %s", sec, opt, val)]++
			}
		}
	}
	fmt.Printf("OptionCounts:\n\n%v\n\n", mapWrap(optionCounts))
	fmt.Printf("ValueCounts:\n\n%v\n\n", mapWrap(valueCounts))
}
