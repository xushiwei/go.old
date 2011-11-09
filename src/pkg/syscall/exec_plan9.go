// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Fork, exec, wait, etc.

package syscall

import (
	"sync"
	"unsafe"
)

// Lock synchronizing creation of new file descriptors with fork.
//
// We want the child in a fork/exec sequence to inherit only the
// file descriptors we intend.  To do that, we mark all file
// descriptors close-on-exec and then, in the child, explicitly
// unmark the ones we want the exec'ed program to keep.
// Unix doesn't make this easy: there is, in general, no way to
// allocate a new file descriptor close-on-exec.  Instead you
// have to allocate the descriptor and then mark it close-on-exec.
// If a fork happens between those two events, the child's exec
// will inherit an unwanted file descriptor.
//
// This lock solves that race: the create new fd/mark close-on-exec
// operation is done holding ForkLock for reading, and the fork itself
// is done holding ForkLock for writing.  At least, that's the idea.
// There are some complications.
//
// Some system calls that create new file descriptors can block
// for arbitrarily long times: open on a hung NFS server or named
// pipe, accept on a socket, and so on.  We can't reasonably grab
// the lock across those operations.
//
// It is worse to inherit some file descriptors than others.
// If a non-malicious child accidentally inherits an open ordinary file,
// that's not a big deal.  On the other hand, if a long-lived child
// accidentally inherits the write end of a pipe, then the reader
// of that pipe will not see EOF until that child exits, potentially
// causing the parent program to hang.  This is a common problem
// in threaded C programs that use popen.
//
// Luckily, the file descriptors that are most important not to
// inherit are not the ones that can take an arbitrarily long time
// to create: pipe returns instantly, and the net package uses
// non-blocking I/O to accept on a listening socket.
// The rules for which file descriptor-creating operations use the
// ForkLock are as follows:
//
// 1) Pipe.    Does not block.  Use the ForkLock.
// 2) Socket.  Does not block.  Use the ForkLock.
// 3) Accept.  If using non-blocking mode, use the ForkLock.
//             Otherwise, live with the race.
// 4) Open.    Can block.  Use O_CLOEXEC if available (Linux).
//             Otherwise, live with the race.
// 5) Dup.     Does not block.  Use the ForkLock.
//             On Linux, could use fcntl F_DUPFD_CLOEXEC
//             instead of the ForkLock, but only for dup(fd, -1).

var ForkLock sync.RWMutex

// Convert array of string to array
// of NUL-terminated byte pointer.
func StringSlicePtr(ss []string) []*byte {
	bb := make([]*byte, len(ss)+1)
	for i := 0; i < len(ss); i++ {
		bb[i] = StringBytePtr(ss[i])
	}
	bb[len(ss)] = nil
	return bb
}

// gbit16 reads a 16-bit numeric value from a 9P protocol message strored in b,
// returning the value and the remaining slice of b.
func gbit16(b []byte) (uint16, []byte) {
	return uint16(b[0]) | uint16(b[1])<<8, b[2:]
}

// gstring reads a string from a 9P protocol message strored in b,
// returning the value as a Go string and the remaining slice of b.
func gstring(b []byte) (string, []byte) {
	n, b := gbit16(b)
	return string(b[0:n]), b[n:]
}

// readdirnames returns the names of files inside the directory represented by dirfd.
func readdirnames(dirfd int) (names []string, err Error) {
	result := make([]string, 0, 100)
	var buf [STATMAX]byte

	for {
		n, e := Read(dirfd, buf[:])
		if e != nil {
			return []string{}, e
		}
		if n == 0 {
			break
		}

		for i := 0; i < n; {
			m, _ := gbit16(buf[i:])
			m += 2

			if m < STATFIXLEN {
				return []string{}, NewError("malformed stat buffer")
			}

			name, _ := gstring(buf[i+41:])
			result = append(result, name)

			i += int(m)
		}
	}
	return []string{}, nil
}

// readdupdevice returns a list of currently opened fds (excluding stdin, stdout, stderr) from the dup device #d.
// ForkLock should be write locked before calling, so that no new fds would be created while the fd list is being read.
func readdupdevice() (fds []int, err Error) {
	dupdevfd, err := Open("#d", O_RDONLY)

	if err != nil {
		return
	}
	defer Close(dupdevfd)

	fileNames, err := readdirnames(dupdevfd)
	if err != nil {
		return
	}

	fds = make([]int, 0, len(fileNames)>>1)
	for _, fdstr := range fileNames {
		if l := len(fdstr); l > 2 && fdstr[l-3] == 'c' && fdstr[l-2] == 't' && fdstr[l-1] == 'l' {
			continue
		}

		fd := int(atoi([]byte(fdstr)))

		if fd == 0 || fd == 1 || fd == 2 || fd == dupdevfd {
			continue
		}

		fds = append(fds, fd)
	}

	return fds[0:len(fds)], nil
}

var startupFds []int

// Plan 9 does not allow clearing the OCEXEC flag
// from the underlying channel backing an open file descriptor,
// therefore we store a list of already opened file descriptors
// inside startupFds and skip them when manually closing descriptors
// not meant to be passed to a child exec.
func init() {
	startupFds, _ = readdupdevice()
}

// forkAndExecInChild forks the process, calling dup onto 0..len(fd)
// and finally invoking exec(argv0, argvv, envv) in the child.
// If a dup or exec fails, it writes the error string to pipe.
// (The pipe write end is close-on-exec so if exec succeeds, it will be closed.)
//
// In the child, this function must not acquire any locks, because
// they might have been locked at the time of the fork.  This means
// no rescheduling, no malloc calls, and no new stack segments.
// The calls to RawSyscall are okay because they are assembly
// functions that do not grow the stack.
func forkAndExecInChild(argv0 *byte, argv []*byte, envv []envItem, dir *byte, attr *ProcAttr, fdsToClose []int, pipe int, rflag int) (pid int, err Error) {
	// Declare all variables at top in case any
	// declarations require heap allocation (e.g., errbuf).
	var (
		r1       uintptr
		nextfd   int
		i        int
		clearenv int
		envfd    int
		errbuf   [ERRMAX]byte
	)

	// guard against side effects of shuffling fds below.
	fd := append([]int(nil), attr.Files...)

	if envv != nil {
		clearenv = RFCENVG
	}

	// About to call fork.
	// No more allocation or calls of non-assembly functions.
	r1, _, _ = RawSyscall(SYS_RFORK, uintptr(RFPROC|RFFDG|RFREND|clearenv|rflag), 0, 0)

	if r1 != 0 {
		if int(r1) == -1 {
			return 0, NewError(errstr())
		}
		// parent; return PID
		return int(r1), nil
	}

	// Fork succeeded, now in child.

	// Close fds we don't need.
	for i = 0; i < len(fdsToClose); i++ {
		r1, _, _ = RawSyscall(SYS_CLOSE, uintptr(fdsToClose[i]), 0, 0)
		if int(r1) == -1 {
			goto childerror
		}
	}

	if envv != nil {
		// Write new environment variables.
		for i = 0; i < len(envv); i++ {
			r1, _, _ = RawSyscall(SYS_CREATE, uintptr(unsafe.Pointer(envv[i].name)), uintptr(O_WRONLY), uintptr(0666))

			if int(r1) == -1 {
				goto childerror
			}

			envfd = int(r1)

			r1, _, _ = RawSyscall6(SYS_PWRITE, uintptr(envfd), uintptr(unsafe.Pointer(envv[i].value)), uintptr(envv[i].nvalue),
				^uintptr(0), ^uintptr(0), 0)

			if int(r1) == -1 || int(r1) != envv[i].nvalue {
				goto childerror
			}

			r1, _, _ = RawSyscall(SYS_CLOSE, uintptr(envfd), 0, 0)

			if int(r1) == -1 {
				goto childerror
			}
		}
	}

	// Chdir
	if dir != nil {
		r1, _, _ = RawSyscall(SYS_CHDIR, uintptr(unsafe.Pointer(dir)), 0, 0)
		if int(r1) == -1 {
			goto childerror
		}
	}

	// Pass 1: look for fd[i] < i and move those up above len(fd)
	// so that pass 2 won't stomp on an fd it needs later.
	nextfd = int(len(fd))
	if pipe < nextfd {
		r1, _, _ = RawSyscall(SYS_DUP, uintptr(pipe), uintptr(nextfd), 0)
		if int(r1) == -1 {
			goto childerror
		}
		pipe = nextfd
		nextfd++
	}
	for i = 0; i < len(fd); i++ {
		if fd[i] >= 0 && fd[i] < int(i) {
			r1, _, _ = RawSyscall(SYS_DUP, uintptr(fd[i]), uintptr(nextfd), 0)
			if int(r1) == -1 {
				goto childerror
			}

			fd[i] = nextfd
			nextfd++
			if nextfd == pipe { // don't stomp on pipe
				nextfd++
			}
		}
	}

	// Pass 2: dup fd[i] down onto i.
	for i = 0; i < len(fd); i++ {
		if fd[i] == -1 {
			RawSyscall(SYS_CLOSE, uintptr(i), 0, 0)
			continue
		}
		if fd[i] == int(i) {
			continue
		}

		r1, _, _ = RawSyscall(SYS_DUP, uintptr(fd[i]), uintptr(i), 0)
		if int(r1) == -1 {
			goto childerror
		}
		RawSyscall(SYS_CLOSE, uintptr(fd[i]), 0, 0)
	}

	// Time to exec.
	r1, _, _ = RawSyscall(SYS_EXEC,
		uintptr(unsafe.Pointer(argv0)),
		uintptr(unsafe.Pointer(&argv[0])), 0)

childerror:
	// send error string on pipe
	RawSyscall(SYS_ERRSTR, uintptr(unsafe.Pointer(&errbuf[0])), uintptr(len(errbuf)), 0)
	errbuf[len(errbuf)-1] = 0
	i = 0
	for i < len(errbuf) && errbuf[i] != 0 {
		i++
	}

	RawSyscall6(SYS_PWRITE, uintptr(pipe), uintptr(unsafe.Pointer(&errbuf[0])), uintptr(i),
		^uintptr(0), ^uintptr(0), 0)

	for {
		RawSyscall(SYS_EXITS, 0, 0, 0)
	}

	// Calling panic is not actually safe,
	// but the for loop above won't break
	// and this shuts up the compiler.
	panic("unreached")
}

func cexecPipe(p []int) Error {
	e := Pipe(p)
	if e != nil {
		return e
	}

	fd, e := Open("#d/"+itoa(p[1]), O_CLOEXEC)
	if e != nil {
		Close(p[0])
		Close(p[1])
		return e
	}

	Close(fd)
	return nil
}

type envItem struct {
	name   *byte
	value  *byte
	nvalue int
}

type ProcAttr struct {
	Dir   string   // Current working directory.
	Env   []string // Environment.
	Files []int    // File descriptors.
	Sys   *SysProcAttr
}

type SysProcAttr struct {
	Rfork int // additional flags to pass to rfork
}

var zeroProcAttr ProcAttr
var zeroSysProcAttr SysProcAttr

func forkExec(argv0 string, argv []string, attr *ProcAttr) (pid int, err Error) {
	var (
		p      [2]int
		n      int
		errbuf [ERRMAX]byte
		wmsg   Waitmsg
	)

	if attr == nil {
		attr = &zeroProcAttr
	}
	sys := attr.Sys
	if sys == nil {
		sys = &zeroSysProcAttr
	}

	p[0] = -1
	p[1] = -1

	// Convert args to C form.
	argv0p := StringBytePtr(argv0)
	argvp := StringSlicePtr(argv)

	var dir *byte
	if attr.Dir != "" {
		dir = StringBytePtr(attr.Dir)
	}
	var envvParsed []envItem
	if attr.Env != nil {
		envvParsed = make([]envItem, 0, len(attr.Env))
		for _, v := range attr.Env {
			i := 0
			for i < len(v) && v[i] != '=' {
				i++
			}

			envvParsed = append(envvParsed, envItem{StringBytePtr("/env/" + v[:i]), StringBytePtr(v[i+1:]), len(v) - i})
		}
	}

	// Acquire the fork lock to prevent other threads from creating new fds before we fork.
	ForkLock.Lock()

	// get a list of open fds, excluding stdin,stdout and stderr that need to be closed in the child.
	// no new fds can be created while we hold the ForkLock for writing.
	openFds, e := readdupdevice()

	if e != nil {
		ForkLock.Unlock()
		return 0, e
	}

	fdsToClose := make([]int, 0, len(openFds))
	// exclude fds opened from startup from the list of fds to be closed.
	for _, fd := range openFds {
		isReserved := false
		for _, reservedFd := range startupFds {
			if fd == reservedFd {
				isReserved = true
				break
			}
		}

		if !isReserved {
			fdsToClose = append(fdsToClose, fd)
		}
	}

	// exclude fds requested by the caller from the list of fds to be closed.
	for _, fd := range openFds {
		isReserved := false
		for _, reservedFd := range attr.Files {
			if fd == reservedFd {
				isReserved = true
				break
			}
		}

		if !isReserved {
			fdsToClose = append(fdsToClose, fd)
		}
	}

	// Allocate child status pipe close on exec.	
	e = cexecPipe(p[:])

	if e != nil {
		return 0, e
	}
	fdsToClose = append(fdsToClose, p[0])

	// Kick off child.
	pid, err = forkAndExecInChild(argv0p, argvp, envvParsed, dir, attr, fdsToClose, p[1], sys.Rfork)

	if err != nil {
		if p[0] >= 0 {
			Close(p[0])
			Close(p[1])
		}
		ForkLock.Unlock()
		return 0, err
	}
	ForkLock.Unlock()

	// Read child error status from pipe.
	Close(p[1])
	n, err = Read(p[0], errbuf[:])
	Close(p[0])

	if err != nil || n != 0 {
		if n != 0 {
			err = NewError(string(errbuf[:]))
		}

		// Child failed; wait for it to exit, to make sure
		// the zombies don't accumulate.
		for wmsg.Pid != pid {
			Await(&wmsg)
		}
		return 0, err
	}

	// Read got EOF, so pipe closed on exec, so exec succeeded.
	return pid, nil
}

// Combination of fork and exec, careful to be thread safe.
func ForkExec(argv0 string, argv []string, attr *ProcAttr) (pid int, err Error) {
	return forkExec(argv0, argv, attr)
}

// StartProcess wraps ForkExec for package os.
func StartProcess(argv0 string, argv []string, attr *ProcAttr) (pid, handle int, err Error) {
	pid, err = forkExec(argv0, argv, attr)
	return pid, 0, err
}

// Ordinary exec.
func Exec(argv0 string, argv []string, envv []string) (err Error) {
	if envv != nil {
		r1, _, _ := RawSyscall(SYS_RFORK, RFCENVG, 0, 0)
		if int(r1) == -1 {
			return NewError(errstr())
		}

		for _, v := range envv {
			i := 0
			for i < len(v) && v[i] != '=' {
				i++
			}

			fd, e := Create("/env/"+v[:i], O_WRONLY, 0666)
			if e != nil {
				return e
			}

			_, e = Write(fd, []byte(v[i+1:]))
			if e != nil {
				Close(fd)
				return e
			}
			Close(fd)
		}
	}

	_, _, e := Syscall(SYS_EXEC,
		uintptr(unsafe.Pointer(StringBytePtr(argv0))),
		uintptr(unsafe.Pointer(&StringSlicePtr(argv)[0])),
		0)

	return NewError(e)
}
