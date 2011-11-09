// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ioutil implements some I/O utility functions.
package ioutil

import (
	"bytes"
	"io"
	"os"
	"sort"
)

// readAll reads from r until an error or EOF and returns the data it read
// from the internal buffer allocated with a specified capacity.
func readAll(r io.Reader, capacity int64) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, capacity))
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

// ReadAll reads from r until an error or EOF and returns the data it read.
func ReadAll(r io.Reader) ([]byte, error) {
	return readAll(r, bytes.MinRead)
}

// ReadFile reads the file named by filename and returns the contents.
func ReadFile(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// It's a good but not certain bet that FileInfo will tell us exactly how much to
	// read, so let's try it but be prepared for the answer to be wrong.
	fi, err := f.Stat()
	var n int64
	if err == nil && fi.Size < 2e9 { // Don't preallocate a huge buffer, just in case.
		n = fi.Size
	}
	// As initial capacity for readAll, use n + a little extra in case Size is zero,
	// and to avoid another allocation after Read has filled the buffer.  The readAll
	// call will read into its allocated internal buffer cheaply.  If the size was
	// wrong, we'll either waste some space off the end or reallocate as needed, but
	// in the overwhelmingly common case we'll get it just right.
	return readAll(f, n+bytes.MinRead)
}

// WriteFile writes data to a file named by filename.
// If the file does not exist, WriteFile creates it with permissions perm;
// otherwise WriteFile truncates it before writing.
func WriteFile(filename string, data []byte, perm uint32) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	f.Close()
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	return err
}

// A fileInfoList implements sort.Interface.
type fileInfoList []*os.FileInfo

func (f fileInfoList) Len() int           { return len(f) }
func (f fileInfoList) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f fileInfoList) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

// ReadDir reads the directory named by dirname and returns
// a list of sorted directory entries.
func ReadDir(dirname string) ([]*os.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	fi := make(fileInfoList, len(list))
	for i := range list {
		fi[i] = &list[i]
	}
	sort.Sort(fi)
	return fi, nil
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

// NopCloser returns a ReadCloser with a no-op Close method wrapping
// the provided Reader r.
func NopCloser(r io.Reader) io.ReadCloser {
	return nopCloser{r}
}

type devNull int

// devNull implements ReaderFrom as an optimization so io.Copy to
// ioutil.Discard can avoid doing unnecessary work.
var _ io.ReaderFrom = devNull(0)

func (devNull) Write(p []byte) (int, error) {
	return len(p), nil
}

var blackHole = make([]byte, 8192)

func (devNull) ReadFrom(r io.Reader) (n int64, err error) {
	readSize := 0
	for {
		readSize, err = r.Read(blackHole)
		n += int64(readSize)
		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			return
		}
	}
	panic("unreachable")
}

// Discard is an io.Writer on which all Write calls succeed
// without doing anything.
var Discard io.Writer = devNull(0)
