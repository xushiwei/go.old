// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

func sortstr(x []string) []string {
	sort.Strings(x)
	return x
}

var buildPkgs = []struct {
	dir  string
	info *DirInfo
}{
	{
		"go/build/pkgtest",
		&DirInfo{
			GoFiles:      []string{"pkgtest.go"},
			SFiles:       []string{"sqrt_" + runtime.GOARCH + ".s"},
			Package:      "pkgtest",
			Imports:      []string{"bytes"},
			TestImports:  []string{"fmt", "pkgtest"},
			TestGoFiles:  sortstr([]string{"sqrt_test.go", "sqrt_" + runtime.GOARCH + "_test.go"}),
			XTestGoFiles: []string{"xsqrt_test.go"},
		},
	},
	{
		"go/build/cmdtest",
		&DirInfo{
			GoFiles:     []string{"main.go"},
			Package:     "main",
			Imports:     []string{"go/build/pkgtest"},
			TestImports: []string{},
		},
	},
	{
		"go/build/cgotest",
		&DirInfo{
			CgoFiles:    ifCgo([]string{"cgotest.go"}),
			CFiles:      []string{"cgotest.c"},
			HFiles:      []string{"cgotest.h"},
			Imports:     []string{"C", "unsafe"},
			TestImports: []string{},
			Package:     "cgotest",
		},
	},
}

func ifCgo(x []string) []string {
	if DefaultContext.CgoEnabled {
		return x
	}
	return nil
}

const cmdtestOutput = "3"

func TestBuild(t *testing.T) {
	for _, tt := range buildPkgs {
		tree := Path[0] // Goroot
		dir := filepath.Join(tree.SrcDir(), tt.dir)
		info, err := ScanDir(dir)
		if err != nil {
			t.Errorf("ScanDir(%#q): %v", tt.dir, err)
			continue
		}
		if !reflect.DeepEqual(info, tt.info) {
			t.Errorf("ScanDir(%#q) = %#v, want %#v\n", tt.dir, info, tt.info)
			continue
		}

		if tt.dir == "go/build/cgotest" && len(info.CgoFiles) == 0 {
			continue
		}

		s, err := Build(tree, tt.dir, info)
		if err != nil {
			t.Errorf("Build(%#q): %v", tt.dir, err)
			continue
		}

		if err := s.Run(); err != nil {
			t.Errorf("Run(%#q): %v", tt.dir, err)
			continue
		}

		if tt.dir == "go/build/cmdtest" {
			bin := s.Output[0]
			b, err := exec.Command(bin).CombinedOutput()
			if err != nil {
				t.Errorf("exec %s: %v", bin, err)
				continue
			}
			if string(b) != cmdtestOutput {
				t.Errorf("cmdtest output: %s want: %s", b, cmdtestOutput)
			}
		}

		// Deferred because cmdtest depends on pkgtest.
		defer func(s *Script) {
			if err := s.Nuke(); err != nil {
				t.Errorf("nuking: %v", err)
			}
		}(s)
	}
}
