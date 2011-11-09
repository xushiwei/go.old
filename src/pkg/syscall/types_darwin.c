// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Input to godefs.  See also mkerrors.sh and mkall.sh
 */

#define __DARWIN_UNIX03 0
#define KERNEL
#define _DARWIN_USE_64_BIT_INODE
#include <dirent.h>
#include <fcntl.h>
#include <signal.h>
#include <unistd.h>
#include <mach/mach.h>
#include <mach/message.h>
#include <sys/event.h>
#include <sys/mman.h>
#include <sys/mount.h>
#include <sys/param.h>
#include <sys/ptrace.h>
#include <sys/resource.h>
#include <sys/select.h>
#include <sys/signal.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <net/bpf.h>
#include <net/if.h>
#include <net/if_dl.h>
#include <net/if_var.h>
#include <net/route.h>
#include <netinet/in.h>
#include <netinet/tcp.h>

// Machine characteristics; for internal use.

enum {
	$sizeofPtr = sizeof(void*),
	$sizeofShort = sizeof(short),
	$sizeofInt = sizeof(int),
	$sizeofLong = sizeof(long),
	$sizeofLongLong = sizeof(long long),
};

// Basic types

typedef short $_C_short;
typedef int $_C_int;
typedef long $_C_long;
typedef long long $_C_long_long;

// Time

typedef struct timespec $Timespec;
typedef struct timeval $Timeval;
typedef struct timeval32 $Timeval32;

// Processes

typedef struct rusage $Rusage;
typedef struct rlimit $Rlimit;

typedef gid_t $_Gid_t;

// Files

enum {
	$O_CLOEXEC = 0,	// not supported
};

typedef struct stat64 $Stat_t;
typedef struct statfs64 $Statfs_t;
typedef struct flock $Flock_t;
typedef struct fstore $Fstore_t;
typedef struct radvisory $Radvisory_t;
typedef struct fbootstraptransfer $Fbootstraptransfer_t;
typedef struct log2phys $Log2phys_t;

typedef struct dirent $Dirent;

// Sockets

union sockaddr_all {
	struct sockaddr s1;	// this one gets used for fields
	struct sockaddr_in s2;	// these pad it out
	struct sockaddr_in6 s3;
	struct sockaddr_un s4;
	struct sockaddr_dl s5;
};

struct sockaddr_any {
	struct sockaddr addr;
	char pad[sizeof(union sockaddr_all) - sizeof(struct sockaddr)];
};

typedef struct sockaddr_in $RawSockaddrInet4;
typedef struct sockaddr_in6 $RawSockaddrInet6;
typedef struct sockaddr_un $RawSockaddrUnix;
typedef struct sockaddr_dl $RawSockaddrDatalink;
typedef struct sockaddr $RawSockaddr;
typedef struct sockaddr_any $RawSockaddrAny;
typedef socklen_t $_Socklen;
typedef struct linger $Linger;
typedef struct iovec $Iovec;
typedef struct ip_mreq $IPMreq;
typedef struct ipv6_mreq $IPv6Mreq;
typedef struct msghdr $Msghdr;
typedef struct cmsghdr $Cmsghdr;
typedef struct in6_pktinfo $Inet6Pktinfo;

enum {
	$SizeofSockaddrInet4 = sizeof(struct sockaddr_in),
	$SizeofSockaddrInet6 = sizeof(struct sockaddr_in6),
	$SizeofSockaddrAny = sizeof(struct sockaddr_any),
	$SizeofSockaddrUnix = sizeof(struct sockaddr_un),
	$SizeofSockaddrDatalink = sizeof(struct sockaddr_dl),
	$SizeofLinger = sizeof(struct linger),
	$SizeofIPMreq = sizeof(struct ip_mreq),
	$SizeofIPv6Mreq = sizeof(struct ipv6_mreq),
	$SizeofMsghdr = sizeof(struct msghdr),
	$SizeofCmsghdr = sizeof(struct cmsghdr),
	$SizeofInet6Pktinfo = sizeof(struct in6_pktinfo),
};

// Ptrace requests

enum {
	$PTRACE_TRACEME = PT_TRACE_ME,
	$PTRACE_CONT = PT_CONTINUE,
	$PTRACE_KILL = PT_KILL,
};

// Events (kqueue, kevent)

typedef struct kevent $Kevent_t;

// Select

typedef fd_set $FdSet;

// Routing and interface messages

enum {
	$SizeofIfMsghdr = sizeof(struct if_msghdr),
	$SizeofIfData = sizeof(struct if_data),
	$SizeofIfaMsghdr = sizeof(struct ifa_msghdr),
	$SizeofIfmaMsghdr = sizeof(struct ifma_msghdr),
	$SizeofIfmaMsghdr2 = sizeof(struct ifma_msghdr2),
	$SizeofRtMsghdr = sizeof(struct rt_msghdr),
	$SizeofRtMetrics = sizeof(struct rt_metrics),
};

typedef struct if_msghdr $IfMsghdr;
typedef struct if_data $IfData;
typedef struct ifa_msghdr $IfaMsghdr;
typedef struct ifma_msghdr $IfmaMsghdr;
typedef struct ifma_msghdr2 $IfmaMsghdr2;
typedef struct rt_msghdr $RtMsghdr;
typedef struct rt_metrics $RtMetrics;

// Berkeley packet filter

enum {
	$SizeofBpfVersion = sizeof(struct bpf_version),
	$SizeofBpfStat = sizeof(struct bpf_stat),
	$SizeofBpfProgram = sizeof(struct bpf_program),
	$SizeofBpfInsn = sizeof(struct bpf_insn),
	$SizeofBpfHdr = sizeof(struct bpf_hdr),
};

typedef struct bpf_version $BpfVersion;
typedef struct bpf_stat $BpfStat;
typedef struct bpf_program $BpfProgram;
typedef struct bpf_insn $BpfInsn;
typedef struct bpf_hdr $BpfHdr;
