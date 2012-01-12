// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd windows

package os

import (
	"syscall"
	"time"
)

func sigpipe() // implemented in package runtime

func epipecheck(file *File, e error) {
	if e == syscall.EPIPE {
		file.nepipe++
		if file.nepipe >= 10 {
			sigpipe()
		}
	} else {
		file.nepipe = 0
	}
}

// LinkError records an error during a link or symlink or rename
// system call and the paths that caused it.
type LinkError struct {
	Op  string
	Old string
	New string
	Err error
}

func (e *LinkError) Error() string {
	return e.Op + " " + e.Old + " " + e.New + ": " + e.Err.Error()
}

// Link creates a hard link.
func Link(oldname, newname string) error {
	e := syscall.Link(oldname, newname)
	if e != nil {
		return &LinkError{"link", oldname, newname, e}
	}
	return nil
}

// Symlink creates a symbolic link.
func Symlink(oldname, newname string) error {
	e := syscall.Symlink(oldname, newname)
	if e != nil {
		return &LinkError{"symlink", oldname, newname, e}
	}
	return nil
}

// Readlink reads the contents of a symbolic link: the destination of
// the link.  It returns the contents and an error, if any.
func Readlink(name string) (string, error) {
	for len := 128; ; len *= 2 {
		b := make([]byte, len)
		n, e := syscall.Readlink(name, b)
		if e != nil {
			return "", &PathError{"readlink", name, e}
		}
		if n < len {
			return string(b[0:n]), nil
		}
	}
	// Silence 6g.
	return "", nil
}

// Rename renames a file.
func Rename(oldname, newname string) error {
	e := syscall.Rename(oldname, newname)
	if e != nil {
		return &LinkError{"rename", oldname, newname, e}
	}
	return nil
}

// Chmod changes the mode of the named file to mode.
// If the file is a symbolic link, it changes the mode of the link's target.
func Chmod(name string, mode uint32) error {
	if e := syscall.Chmod(name, mode); e != nil {
		return &PathError{"chmod", name, e}
	}
	return nil
}

// Chmod changes the mode of the file to mode.
func (f *File) Chmod(mode uint32) error {
	if e := syscall.Fchmod(f.fd, mode); e != nil {
		return &PathError{"chmod", f.name, e}
	}
	return nil
}

// Chown changes the numeric uid and gid of the named file.
// If the file is a symbolic link, it changes the uid and gid of the link's target.
func Chown(name string, uid, gid int) error {
	if e := syscall.Chown(name, uid, gid); e != nil {
		return &PathError{"chown", name, e}
	}
	return nil
}

// Lchown changes the numeric uid and gid of the named file.
// If the file is a symbolic link, it changes the uid and gid of the link itself.
func Lchown(name string, uid, gid int) error {
	if e := syscall.Lchown(name, uid, gid); e != nil {
		return &PathError{"lchown", name, e}
	}
	return nil
}

// Chown changes the numeric uid and gid of the named file.
func (f *File) Chown(uid, gid int) error {
	if e := syscall.Fchown(f.fd, uid, gid); e != nil {
		return &PathError{"chown", f.name, e}
	}
	return nil
}

// Truncate changes the size of the file.
// It does not change the I/O offset.
func (f *File) Truncate(size int64) error {
	if e := syscall.Ftruncate(f.fd, size); e != nil {
		return &PathError{"truncate", f.name, e}
	}
	return nil
}

// Sync commits the current contents of the file to stable storage.
// Typically, this means flushing the file system's in-memory copy
// of recently written data to disk.
func (f *File) Sync() (err error) {
	if f == nil {
		return EINVAL
	}
	if e := syscall.Fsync(f.fd); e != nil {
		return NewSyscallError("fsync", e)
	}
	return nil
}

// Chtimes changes the access and modification times of the named
// file, similar to the Unix utime() or utimes() functions.
//
// The underlying filesystem may truncate or round the values to a
// less precise time unit.
func Chtimes(name string, atime time.Time, mtime time.Time) error {
	var utimes [2]syscall.Timeval
	atime_ns := atime.Unix()*1e9 + int64(atime.Nanosecond())
	mtime_ns := mtime.Unix()*1e9 + int64(mtime.Nanosecond())
	utimes[0] = syscall.NsecToTimeval(atime_ns)
	utimes[1] = syscall.NsecToTimeval(mtime_ns)
	if e := syscall.Utimes(name, utimes[0:]); e != nil {
		return &PathError{"chtimes", name, e}
	}
	return nil
}
