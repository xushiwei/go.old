// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	codeProject      = "go"
	codePyScript     = "misc/dashboard/googlecode_upload.py"
	hgUrl            = "https://go.googlecode.com/hg/"
	waitInterval     = 30e9 // time to wait before checking for new revs
	mkdirPerm        = 0750
	pkgBuildInterval = 1e9 * 60 * 60 * 24 // rebuild packages every 24 hours
)

// These variables are copied from the gobuilder's environment
// to the envv of its subprocesses.
var extraEnv = []string{
	"GOHOSTOS",
	"GOHOSTARCH",
	"PATH",
	"DISABLE_NET_TESTS",
	"MAKEFLAGS",
	"GOARM",
}

type Builder struct {
	name         string
	goos, goarch string
	key          string
	codeUsername string
	codePassword string
}

var (
	buildroot     = flag.String("buildroot", path.Join(os.TempDir(), "gobuilder"), "Directory under which to build")
	commitFlag    = flag.Bool("commit", false, "upload information about new commits")
	dashboard     = flag.String("dashboard", "godashboard.appspot.com", "Go Dashboard Host")
	buildRelease  = flag.Bool("release", false, "Build and upload binary release archives")
	buildRevision = flag.String("rev", "", "Build specified revision and exit")
	buildCmd      = flag.String("cmd", "./all.bash", "Build command (specify absolute or relative to go/src/)")
	external      = flag.Bool("external", false, "Build external packages")
	parallel      = flag.Bool("parallel", false, "Build multiple targets in parallel")
	verbose       = flag.Bool("v", false, "verbose")
)

var (
	goroot      string
	binaryTagRe = regexp.MustCompile(`^(release\.r|weekly\.)[0-9\-.]+`)
	releaseRe   = regexp.MustCompile(`^release\.r[0-9\-.]+`)
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s goos-goarch...\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if len(flag.Args()) == 0 && !*commitFlag {
		flag.Usage()
	}
	goroot = path.Join(*buildroot, "goroot")
	builders := make([]*Builder, len(flag.Args()))
	for i, builder := range flag.Args() {
		b, err := NewBuilder(builder)
		if err != nil {
			log.Fatal(err)
		}
		builders[i] = b
	}

	// set up work environment
	if err := os.RemoveAll(*buildroot); err != nil {
		log.Fatalf("Error removing build root (%s): %s", *buildroot, err)
	}
	if err := os.Mkdir(*buildroot, mkdirPerm); err != nil {
		log.Fatalf("Error making build root (%s): %s", *buildroot, err)
	}
	if err := run(nil, *buildroot, "hg", "clone", hgUrl, goroot); err != nil {
		log.Fatal("Error cloning repository:", err)
	}

	if *commitFlag {
		if len(flag.Args()) == 0 {
			commitWatcher()
			return
		}
		go commitWatcher()
	}

	// if specified, build revision and return
	if *buildRevision != "" {
		hash, err := fullHash(*buildRevision)
		if err != nil {
			log.Fatal("Error finding revision: ", err)
		}
		for _, b := range builders {
			if err := b.buildHash(hash); err != nil {
				log.Println(err)
			}
		}
		return
	}

	// external package build mode
	if *external {
		if len(builders) != 1 {
			log.Fatal("only one goos-goarch should be specified with -external")
		}
		builders[0].buildExternal()
	}

	// go continuous build mode (default)
	// check for new commits and build them
	for {
		built := false
		t := time.Nanoseconds()
		if *parallel {
			done := make(chan bool)
			for _, b := range builders {
				go func(b *Builder) {
					done <- b.build()
				}(b)
			}
			for _ = range builders {
				built = <-done || built
			}
		} else {
			for _, b := range builders {
				built = b.build() || built
			}
		}
		// sleep if there was nothing to build
		if !built {
			time.Sleep(waitInterval)
		}
		// sleep if we're looping too fast.
		t1 := time.Nanoseconds() - t
		if t1 < waitInterval {
			time.Sleep(waitInterval - t1)
		}
	}
}

func NewBuilder(builder string) (*Builder, error) {
	b := &Builder{name: builder}

	// get goos/goarch from builder string
	s := strings.SplitN(builder, "-", 3)
	if len(s) >= 2 {
		b.goos, b.goarch = s[0], s[1]
	} else {
		return nil, fmt.Errorf("unsupported builder form: %s", builder)
	}

	// read keys from keyfile
	fn := path.Join(os.Getenv("HOME"), ".gobuildkey")
	if s := fn + "-" + b.name; isFile(s) { // builder-specific file
		fn = s
	}
	c, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("readKeys %s (%s): %s", b.name, fn, err)
	}
	v := strings.Split(string(c), "\n")
	b.key = v[0]
	if len(v) >= 3 {
		b.codeUsername, b.codePassword = v[1], v[2]
	}

	return b, nil
}

// buildExternal downloads and builds external packages, and
// reports their build status to the dashboard.
// It will re-build all packages after pkgBuildInterval nanoseconds or
// a new release tag is found.
func (b *Builder) buildExternal() {
	var prevTag string
	var nextBuild int64
	for {
		time.Sleep(waitInterval)
		err := run(nil, goroot, "hg", "pull", "-u")
		if err != nil {
			log.Println("hg pull failed:", err)
			continue
		}
		hash, tag, err := firstTag(releaseRe)
		if err != nil {
			log.Println(err)
			continue
		}
		if *verbose {
			log.Println("latest release:", tag)
		}
		// don't rebuild if there's no new release
		// and it's been less than pkgBuildInterval
		// nanoseconds since the last build.
		if tag == prevTag && time.Nanoseconds() < nextBuild {
			continue
		}
		// build will also build the packages
		if err := b.buildHash(hash); err != nil {
			log.Println(err)
			continue
		}
		prevTag = tag
		nextBuild = time.Nanoseconds() + pkgBuildInterval
	}
}

// build checks for a new commit for this builder
// and builds it if one is found. 
// It returns true if a build was attempted.
func (b *Builder) build() bool {
	defer func() {
		err := recover()
		if err != nil {
			log.Println(b.name, "build:", err)
		}
	}()
	hash, err := b.todo()
	if err != nil {
		log.Println(err)
		return false
	}
	if hash == "" {
		return false
	}
	// Look for hash locally before running hg pull.

	if _, err := fullHash(hash[:12]); err != nil {
		// Don't have hash, so run hg pull.
		if err := run(nil, goroot, "hg", "pull"); err != nil {
			log.Println("hg pull failed:", err)
			return false
		}
	}
	err = b.buildHash(hash)
	if err != nil {
		log.Println(err)
	}
	return true
}

func (b *Builder) buildHash(hash string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%s build: %s: %s", b.name, hash, err)
		}
	}()

	log.Println(b.name, "building", hash)

	// create place in which to do work
	workpath := path.Join(*buildroot, b.name+"-"+hash[:12])
	err = os.Mkdir(workpath, mkdirPerm)
	if err != nil {
		return
	}
	defer os.RemoveAll(workpath)

	// clone repo
	err = run(nil, workpath, "hg", "clone", goroot, "go")
	if err != nil {
		return
	}

	// update to specified revision
	err = run(nil, path.Join(workpath, "go"),
		"hg", "update", hash)
	if err != nil {
		return
	}

	srcDir := path.Join(workpath, "go", "src")

	// build
	logfile := path.Join(workpath, "build.log")
	buildLog, status, err := runLog(b.envv(), logfile, srcDir, *buildCmd)
	if err != nil {
		return fmt.Errorf("%s: %s", *buildCmd, err)
	}

	// if we're in external mode, build all packages and return
	if *external {
		if status != 0 {
			return errors.New("go build failed")
		}
		return b.buildPackages(workpath, hash)
	}

	if status != 0 {
		// record failure
		return b.recordResult(buildLog, hash)
	}

	// record success
	if err = b.recordResult("", hash); err != nil {
		return fmt.Errorf("recordResult: %s", err)
	}

	// finish here if codeUsername and codePassword aren't set
	if b.codeUsername == "" || b.codePassword == "" || !*buildRelease {
		return
	}

	// if this is a release, create tgz and upload to google code
	releaseHash, release, err := firstTag(binaryTagRe)
	if hash == releaseHash {
		// clean out build state
		err = run(b.envv(), srcDir, "./clean.bash", "--nopkg")
		if err != nil {
			return fmt.Errorf("clean.bash: %s", err)
		}
		// upload binary release
		fn := fmt.Sprintf("go.%s.%s-%s.tar.gz", release, b.goos, b.goarch)
		err = run(nil, workpath, "tar", "czf", fn, "go")
		if err != nil {
			return fmt.Errorf("tar: %s", err)
		}
		err = run(nil, workpath, path.Join(goroot, codePyScript),
			"-s", release,
			"-p", codeProject,
			"-u", b.codeUsername,
			"-w", b.codePassword,
			"-l", fmt.Sprintf("%s,%s", b.goos, b.goarch),
			fn)
	}

	return
}

// envv returns an environment for build/bench execution
func (b *Builder) envv() []string {
	if runtime.GOOS == "windows" {
		return b.envvWindows()
	}
	e := []string{
		"GOOS=" + b.goos,
		"GOARCH=" + b.goarch,
		"GOROOT_FINAL=/usr/local/go",
	}
	for _, k := range extraEnv {
		s, err := os.Getenverror(k)
		if err == nil {
			e = append(e, k+"="+s)
		}
	}
	return e
}

// windows version of envv
func (b *Builder) envvWindows() []string {
	start := map[string]string{
		"GOOS":         b.goos,
		"GOARCH":       b.goarch,
		"GOROOT_FINAL": "/c/go",
		// TODO(brainman): remove once we find make that does not hang.
		"MAKEFLAGS": "-j1",
	}
	for _, name := range extraEnv {
		s, err := os.Getenverror(name)
		if err == nil {
			start[name] = s
		}
	}
	skip := map[string]bool{
		"GOBIN":   true,
		"GOROOT":  true,
		"INCLUDE": true,
		"LIB":     true,
	}
	var e []string
	for name, v := range start {
		e = append(e, name+"="+v)
		skip[name] = true
	}
	for _, kv := range os.Environ() {
		s := strings.SplitN(kv, "=", 2)
		name := strings.ToUpper(s[0])
		switch {
		case name == "":
			// variables, like "=C:=C:\", just copy them
			e = append(e, kv)
		case !skip[name]:
			e = append(e, kv)
			skip[name] = true
		}
	}
	return e
}

func isDirectory(name string) bool {
	s, err := os.Stat(name)
	return err == nil && s.IsDirectory()
}

func isFile(name string) bool {
	s, err := os.Stat(name)
	return err == nil && (s.IsRegular() || s.IsSymlink())
}

// commitWatcher polls hg for new commits and tells the dashboard about them.
func commitWatcher() {
	// Create builder just to get master key.
	b, err := NewBuilder("mercurial-commit")
	if err != nil {
		log.Fatal(err)
	}
	for {
		if *verbose {
			log.Printf("poll...")
		}
		commitPoll(b.key)
		if *verbose {
			log.Printf("sleep...")
		}
		time.Sleep(60e9)
	}
}

// HgLog represents a single Mercurial revision.
type HgLog struct {
	Hash   string
	Author string
	Date   string
	Desc   string
	Parent string

	// Internal metadata
	added bool
}

// logByHash is a cache of all Mercurial revisions we know about,
// indexed by full hash.
var logByHash = map[string]*HgLog{}

// xmlLogTemplate is a template to pass to Mercurial to make
// hg log print the log in valid XML for parsing with xml.Unmarshal.
const xmlLogTemplate = `
	<log>
	<hash>{node|escape}</hash>
	<parent>{parent|escape}</parent>
	<author>{author|escape}</author>
	<date>{date}</date>
	<desc>{desc|escape}</desc>
	</log>
`

// commitPoll pulls any new revisions from the hg server
// and tells the server about them.
func commitPoll(key string) {
	// Catch unexpected panics.
	defer func() {
		if err := recover(); err != nil {
			log.Printf("commitPoll panic: %s", err)
		}
	}()

	if err := run(nil, goroot, "hg", "pull"); err != nil {
		log.Printf("hg pull: %v", err)
		return
	}

	const N = 50 // how many revisions to grab

	data, _, err := runLog(nil, "", goroot, "hg", "log",
		"--encoding=utf-8",
		"--limit="+strconv.Itoa(N),
		"--template="+xmlLogTemplate,
	)
	if err != nil {
		log.Printf("hg log: %v", err)
		return
	}

	var logStruct struct {
		Log []HgLog
	}
	err = xml.Unmarshal(strings.NewReader("<top>"+data+"</top>"), &logStruct)
	if err != nil {
		log.Printf("unmarshal hg log: %v", err)
		return
	}

	logs := logStruct.Log

	// Pass 1.  Fill in parents and add new log entries to logsByHash.
	// Empty parent means take parent from next log entry.
	// Non-empty parent has form 1234:hashhashhash; we want full hash.
	for i := range logs {
		l := &logs[i]
		if l.Parent == "" && i+1 < len(logs) {
			l.Parent = logs[i+1].Hash
		} else if l.Parent != "" {
			l.Parent, _ = fullHash(l.Parent)
		}
		log.Printf("hg log: %s < %s\n", l.Hash, l.Parent)
		if l.Parent == "" {
			// Can't create node without parent.
			continue
		}

		if logByHash[l.Hash] == nil {
			// Make copy to avoid pinning entire slice when only one entry is new.
			t := *l
			logByHash[t.Hash] = &t
		}
	}

	for i := range logs {
		l := &logs[i]
		if l.Parent == "" {
			continue
		}
		addCommit(l.Hash, key)
	}
}

// addCommit adds the commit with the named hash to the dashboard.
// key is the secret key for authentication to the dashboard.
// It avoids duplicate effort.
func addCommit(hash, key string) bool {
	l := logByHash[hash]
	if l == nil {
		return false
	}
	if l.added {
		return true
	}

	// Check for already added, perhaps in an earlier run.
	if dashboardCommit(hash) {
		log.Printf("%s already on dashboard\n", hash)
		// Record that this hash is on the dashboard,
		// as must be all its parents.
		for l != nil {
			l.added = true
			l = logByHash[l.Parent]
		}
		return true
	}

	// Create parent first, to maintain some semblance of order.
	if !addCommit(l.Parent, key) {
		return false
	}

	// Create commit.
	if err := postCommit(key, l); err != nil {
		log.Printf("failed to add %s to dashboard: %v", key, err)
		return false
	}
	return true
}

// fullHash returns the full hash for the given Mercurial revision.
func fullHash(rev string) (hash string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("fullHash: %s: %s", rev, err)
		}
	}()
	s, _, err := runLog(nil, "", goroot,
		"hg", "log",
		"--encoding=utf-8",
		"--rev="+rev,
		"--limit=1",
		"--template={node}",
	)
	if err != nil {
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("cannot find revision")
	}
	if len(s) != 40 {
		return "", fmt.Errorf("hg returned invalid hash " + s)
	}
	return s, nil
}

var revisionRe = regexp.MustCompile(`^([^ ]+) +[0-9]+:([0-9a-f]+)$`)

// firstTag returns the hash and tag of the most recent tag matching re.
func firstTag(re *regexp.Regexp) (hash string, tag string, err error) {
	o, _, err := runLog(nil, "", goroot, "hg", "tags")
	for _, l := range strings.Split(o, "\n") {
		if l == "" {
			continue
		}
		s := revisionRe.FindStringSubmatch(l)
		if s == nil {
			err = errors.New("couldn't find revision number")
			return
		}
		if !re.MatchString(s[1]) {
			continue
		}
		tag = s[1]
		hash, err = fullHash(s[2])
		return
	}
	err = errors.New("no matching tag found")
	return
}
