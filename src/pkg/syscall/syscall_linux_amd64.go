// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

//sys	Chown(path string, uid int, gid int) (errno int)
//sys	Fchown(fd int, uid int, gid int) (errno int)
//sys	Fstat(fd int, stat *Stat_t) (errno int)
//sys	Fstatfs(fd int, buf *Statfs_t) (errno int)
//sys	Ftruncate(fd int, length int64) (errno int)
//sysnb	Getegid() (egid int)
//sysnb	Geteuid() (euid int)
//sysnb	Getgid() (gid int)
//sysnb	Getuid() (uid int)
//sys	Ioperm(from int, num int, on int) (errno int)
//sys	Iopl(level int) (errno int)
//sys	Lchown(path string, uid int, gid int) (errno int)
//sys	Listen(s int, n int) (errno int)
//sys	Lstat(path string, stat *Stat_t) (errno int)
//sys	Pread(fd int, p []byte, offset int64) (n int, errno int) = SYS_PREAD64
//sys	Pwrite(fd int, p []byte, offset int64) (n int, errno int) = SYS_PWRITE64
//sys	Seek(fd int, offset int64, whence int) (off int64, errno int) = SYS_LSEEK
//sys	Select(nfd int, r *FdSet, w *FdSet, e *FdSet, timeout *Timeval) (n int, errno int)
//sys	Sendfile(outfd int, infd int, offset *int64, count int) (written int, errno int)
//sys	Setfsgid(gid int) (errno int)
//sys	Setfsuid(uid int) (errno int)
//sysnb	Setgid(gid int) (errno int)
//sysnb	Setregid(rgid int, egid int) (errno int)
//sysnb	Setresgid(rgid int, egid int, sgid int) (errno int)
//sysnb	Setresuid(ruid int, euid int, suid int) (errno int)
//sysnb	Setreuid(ruid int, euid int) (errno int)
//sys	Shutdown(fd int, how int) (errno int)
//sys	Splice(rfd int, roff *int64, wfd int, woff *int64, len int, flags int) (n int64, errno int)
//sys	Stat(path string, stat *Stat_t) (errno int)
//sys	Statfs(path string, buf *Statfs_t) (errno int)
//sys	SyncFileRange(fd int, off int64, n int64, flags int) (errno int)
//sys	Truncate(path string, length int64) (errno int)
//sys	accept(s int, rsa *RawSockaddrAny, addrlen *_Socklen) (fd int, errno int)
//sys	bind(s int, addr uintptr, addrlen _Socklen) (errno int)
//sys	connect(s int, addr uintptr, addrlen _Socklen) (errno int)
//sysnb	getgroups(n int, list *_Gid_t) (nn int, errno int)
//sysnb	setgroups(n int, list *_Gid_t) (errno int)
//sys	getsockopt(s int, level int, name int, val uintptr, vallen *_Socklen) (errno int)
//sys	setsockopt(s int, level int, name int, val uintptr, vallen uintptr) (errno int)
//sysnb	socket(domain int, typ int, proto int) (fd int, errno int)
//sysnb	socketpair(domain int, typ int, proto int, fd *[2]int) (errno int)
//sysnb	getpeername(fd int, rsa *RawSockaddrAny, addrlen *_Socklen) (errno int)
//sysnb	getsockname(fd int, rsa *RawSockaddrAny, addrlen *_Socklen) (errno int)
//sys	recvfrom(fd int, p []byte, flags int, from *RawSockaddrAny, fromlen *_Socklen) (n int, errno int)
//sys	sendto(s int, buf []byte, flags int, to uintptr, addrlen _Socklen) (errno int)
//sys	recvmsg(s int, msg *Msghdr, flags int) (n int, errno int)
//sys	sendmsg(s int, msg *Msghdr, flags int) (errno int)
//sys	mmap(addr uintptr, length uintptr, prot int, flags int, fd int, offset int64) (xaddr uintptr, errno int)

func Getpagesize() int { return 4096 }

func Gettimeofday(tv *Timeval) (errno int)
func Time(t *Time_t) (tt Time_t, errno int)

func TimespecToNsec(ts Timespec) int64 { return int64(ts.Sec)*1e9 + int64(ts.Nsec) }

func NsecToTimespec(nsec int64) (ts Timespec) {
	ts.Sec = nsec / 1e9
	ts.Nsec = nsec % 1e9
	return
}

func TimevalToNsec(tv Timeval) int64 { return int64(tv.Sec)*1e9 + int64(tv.Usec)*1e3 }

func NsecToTimeval(nsec int64) (tv Timeval) {
	nsec += 999 // round up to microsecond
	tv.Sec = nsec / 1e9
	tv.Usec = nsec % 1e9 / 1e3
	return
}

func (r *PtraceRegs) PC() uint64 { return r.Rip }

func (r *PtraceRegs) SetPC(pc uint64) { r.Rip = pc }

func (iov *Iovec) SetLen(length int) {
	iov.Len = uint64(length)
}

func (msghdr *Msghdr) SetControllen(length int) {
	msghdr.Controllen = uint64(length)
}

func (cmsg *Cmsghdr) SetLen(length int) {
	cmsg.Len = uint64(length)
}
