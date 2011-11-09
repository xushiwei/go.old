// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"syscall"
	"unsafe"
)

// Stat returns the FileInfo structure describing file.
// It returns the FileInfo and an error, if any.
func (file *File) Stat() (fi *FileInfo, err error) {
	if file == nil || file.fd < 0 {
		return nil, EINVAL
	}
	if file.isdir() {
		// I don't know any better way to do that for directory
		return Stat(file.name)
	}
	var d syscall.ByHandleFileInformation
	e := syscall.GetFileInformationByHandle(syscall.Handle(file.fd), &d)
	if e != 0 {
		return nil, &PathError{"GetFileInformationByHandle", file.name, Errno(e)}
	}
	return setFileInfo(new(FileInfo), basename(file.name), d.FileAttributes, d.FileSizeHigh, d.FileSizeLow, d.CreationTime, d.LastAccessTime, d.LastWriteTime), nil
}

// Stat returns a FileInfo structure describing the named file and an error, if any.
// If name names a valid symbolic link, the returned FileInfo describes
// the file pointed at by the link and has fi.FollowedSymlink set to true.
// If name names an invalid symbolic link, the returned FileInfo describes
// the link itself and has fi.FollowedSymlink set to false.
func Stat(name string) (fi *FileInfo, err error) {
	if len(name) == 0 {
		return nil, &PathError{"Stat", name, Errno(syscall.ERROR_PATH_NOT_FOUND)}
	}
	var d syscall.Win32FileAttributeData
	e := syscall.GetFileAttributesEx(syscall.StringToUTF16Ptr(name), syscall.GetFileExInfoStandard, (*byte)(unsafe.Pointer(&d)))
	if e != 0 {
		return nil, &PathError{"GetFileAttributesEx", name, Errno(e)}
	}
	return setFileInfo(new(FileInfo), basename(name), d.FileAttributes, d.FileSizeHigh, d.FileSizeLow, d.CreationTime, d.LastAccessTime, d.LastWriteTime), nil
}

// Lstat returns the FileInfo structure describing the named file and an
// error, if any.  If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
func Lstat(name string) (fi *FileInfo, err error) {
	// No links on Windows
	return Stat(name)
}

// basename removes trailing slashes and the leading
// directory name and drive letter from path name.
func basename(name string) string {
	// Remove drive letter
	if len(name) == 2 && name[1] == ':' {
		name = "."
	} else if len(name) > 2 && name[1] == ':' {
		name = name[2:]
	}
	i := len(name) - 1
	// Remove trailing slashes
	for ; i > 0 && (name[i] == '/' || name[i] == '\\'); i-- {
		name = name[:i]
	}
	// Remove leading directory name
	for i--; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			name = name[i+1:]
			break
		}
	}
	return name
}

func setFileInfo(fi *FileInfo, name string, fa, sizehi, sizelo uint32, ctime, atime, wtime syscall.Filetime) *FileInfo {
	fi.Mode = 0
	if fa&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		fi.Mode = fi.Mode | syscall.S_IFDIR
	} else {
		fi.Mode = fi.Mode | syscall.S_IFREG
	}
	if fa&syscall.FILE_ATTRIBUTE_READONLY != 0 {
		fi.Mode = fi.Mode | 0444
	} else {
		fi.Mode = fi.Mode | 0666
	}
	fi.Size = int64(sizehi)<<32 + int64(sizelo)
	fi.Name = name
	fi.FollowedSymlink = false
	fi.Atime_ns = atime.Nanoseconds()
	fi.Mtime_ns = wtime.Nanoseconds()
	fi.Ctime_ns = ctime.Nanoseconds()
	return fi
}
