// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"patch"
	"path/filepath"
	"sort"
	"strings"
)

var checkSync = flag.Bool("checksync", true, "check whether repository is out of sync")

func usage() {
	fmt.Fprintf(os.Stderr, "usage: hgpatch [options] [patchfile]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	var data []byte
	var err error
	switch len(args) {
	case 0:
		data, err = ioutil.ReadAll(os.Stdin)
	case 1:
		data, err = ioutil.ReadFile(args[0])
	default:
		usage()
	}
	chk(err)

	pset, err := patch.Parse(data)
	chk(err)

	// Change to hg root directory, because
	// patch paths are relative to root.
	root, err := hgRoot()
	chk(err)
	chk(os.Chdir(root))

	// Make sure there are no pending changes on the server.
	if *checkSync && hgIncoming() {
		fmt.Fprintf(os.Stderr, "incoming changes waiting; run hg sync first\n")
		os.Exit(2)
	}

	// Make sure we won't be editing files with local pending changes.
	dirtylist, err := hgModified()
	chk(err)
	dirty := make(map[string]bool)
	for _, f := range dirtylist {
		dirty[f] = true
	}
	conflict := make(map[string]bool)
	for _, f := range pset.File {
		if f.Verb == patch.Delete || f.Verb == patch.Rename {
			if dirty[f.Src] {
				conflict[f.Src] = true
			}
		}
		if f.Verb != patch.Delete {
			if dirty[f.Dst] {
				conflict[f.Dst] = true
			}
		}
	}
	if len(conflict) > 0 {
		fmt.Fprintf(os.Stderr, "cannot apply patch to locally modified files:\n")
		for name := range conflict {
			fmt.Fprintf(os.Stderr, "\t%s\n", name)
		}
		os.Exit(2)
	}

	// Apply changes in memory.
	op, err := pset.Apply(ioutil.ReadFile)
	chk(err)

	// Write changes to disk copy: order of commands matters.
	// Accumulate undo log as we go, in case there is an error.
	// Also accumulate list of modified files to print at end.
	changed := make(map[string]int)

	// Copy, Rename create the destination file, so they
	// must happen before we write the data out.
	// A single patch may have a Copy and a Rename
	// with the same source, so we have to run all the
	// Copy in one pass, then all the Rename.
	for i := range op {
		o := &op[i]
		if o.Verb == patch.Copy {
			makeParent(o.Dst)
			chk(hgCopy(o.Dst, o.Src))
			undoRevert(o.Dst)
			changed[o.Dst] = 1
		}
	}
	for i := range op {
		o := &op[i]
		if o.Verb == patch.Rename {
			makeParent(o.Dst)
			chk(hgRename(o.Dst, o.Src))
			undoRevert(o.Dst)
			undoRevert(o.Src)
			changed[o.Src] = 1
			changed[o.Dst] = 1
		}
	}

	// Run Delete before writing to files in case one of the
	// deleted paths is becoming a directory.
	for i := range op {
		o := &op[i]
		if o.Verb == patch.Delete {
			chk(hgRemove(o.Src))
			undoRevert(o.Src)
			changed[o.Src] = 1
		}
	}

	// Write files.
	for i := range op {
		o := &op[i]
		if o.Verb == patch.Delete {
			continue
		}
		if o.Verb == patch.Add {
			makeParent(o.Dst)
			changed[o.Dst] = 1
		}
		if o.Data != nil {
			chk(ioutil.WriteFile(o.Dst, o.Data, 0644))
			if o.Verb == patch.Add {
				undoRm(o.Dst)
			} else {
				undoRevert(o.Dst)
			}
			changed[o.Dst] = 1
		}
		if o.Mode != 0 {
			chk(os.Chmod(o.Dst, uint32(o.Mode&0755)))
			undoRevert(o.Dst)
			changed[o.Dst] = 1
		}
	}

	// hg add looks at the destination file, so it must happen
	// after we write the data out.
	for i := range op {
		o := &op[i]
		if o.Verb == patch.Add {
			chk(hgAdd(o.Dst))
			undoRevert(o.Dst)
			changed[o.Dst] = 1
		}
	}

	// Finished editing files.  Write the list of changed files to stdout.
	list := make([]string, len(changed))
	i := 0
	for f := range changed {
		list[i] = f
		i++
	}
	sort.Strings(list)
	for _, f := range list {
		fmt.Printf("%s\n", f)
	}
}

// make parent directory for name, if necessary
func makeParent(name string) {
	parent, _ := filepath.Split(name)
	chk(mkdirAll(parent, 0755))
}

// Copy of os.MkdirAll but adds to undo log after
// creating a directory.
func mkdirAll(path string, perm uint32) error {
	dir, err := os.Lstat(path)
	if err == nil {
		if dir.IsDirectory() {
			return nil
		}
		return &os.PathError{"mkdir", path, os.ENOTDIR}
	}

	i := len(path)
	for i > 0 && path[i-1] == '/' { // Skip trailing slashes.
		i--
	}

	j := i
	for j > 0 && path[j-1] != '/' { // Scan backward over element.
		j--
	}

	if j > 0 {
		err = mkdirAll(path[0:j-1], perm)
		if err != nil {
			return err
		}
	}

	err = os.Mkdir(path, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := os.Lstat(path)
		if err1 == nil && dir.IsDirectory() {
			return nil
		}
		return err
	}
	undoRm(path)
	return nil
}

// If err != nil, process the undo log and exit.
func chk(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		runUndo()
		os.Exit(2)
	}
}

// Undo log
type undo func() error

var undoLog []undo

func undoRevert(name string) {
	undoLog = append(undoLog, undo(func() error { return hgRevert(name) }))
}

func undoRm(name string) { undoLog = append(undoLog, undo(func() error { return os.Remove(name) })) }

func runUndo() {
	for i := len(undoLog) - 1; i >= 0; i-- {
		if err := undoLog[i](); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
	}
}

// hgRoot returns the root directory of the repository.
func hgRoot() (string, error) {
	out, err := run([]string{"hg", "root"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// hgIncoming returns true if hg sync will pull in changes.
func hgIncoming() bool {
	// Cannot trust hg's exit code on Windows,
	// so look at whether hg prints any output.
	out, _ := run([]string{"hg", "-q", "incoming"}, nil)
	return len(out) > 0
}

// hgModified returns a list of the modified files in the
// repository.
func hgModified() ([]string, error) {
	out, err := run([]string{"hg", "status", "-n"}, nil)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(out), "\n"), nil
}

// hgAdd adds name to the repository.
func hgAdd(name string) error {
	_, err := run([]string{"hg", "add", name}, nil)
	return err
}

// hgRemove removes name from the repository.
func hgRemove(name string) error {
	_, err := run([]string{"hg", "rm", name}, nil)
	return err
}

// hgRevert reverts name.
func hgRevert(name string) error {
	_, err := run([]string{"hg", "revert", name}, nil)
	return err
}

// hgCopy copies src to dst in the repository.
// Note that the argument order matches io.Copy, not "hg cp".
func hgCopy(dst, src string) error {
	_, err := run([]string{"hg", "cp", src, dst}, nil)
	return err
}

// hgRename renames src to dst in the repository.
// Note that the argument order matches io.Copy, not "hg mv".
func hgRename(dst, src string) error {
	_, err := run([]string{"hg", "mv", src, dst}, nil)
	return err
}

func dup(a []string) []string {
	b := make([]string, len(a))
	copy(b, a)
	return b
}

var lookPathCache = make(map[string]string)

// run runs the command argv, resolving argv[0] if necessary by searching $PATH.
// It provides input on standard input to the command.
func run(argv []string, input []byte) (out string, err error) {
	if len(argv) < 1 {
		return "", &runError{dup(argv), os.EINVAL}
	}

	prog, ok := lookPathCache[argv[0]]
	if !ok {
		prog, err = exec.LookPath(argv[0])
		if err != nil {
			return "", &runError{dup(argv), err}
		}
		lookPathCache[argv[0]] = prog
	}

	cmd := exec.Command(prog, argv[1:]...)
	if len(input) > 0 {
		cmd.Stdin = bytes.NewBuffer(input)
	}
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return "", &runError{dup(argv), err}
	}
	return string(bs), nil
}

// A runError represents an error that occurred while running a command.
type runError struct {
	cmd []string
	err error
}

func (e *runError) Error() string { return strings.Join(e.cmd, " ") + ": " + e.err.Error() }
