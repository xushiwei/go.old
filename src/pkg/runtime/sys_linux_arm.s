// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//
// System calls and other sys.stuff for arm, Linux
//

#include "zasm_GOOS_GOARCH.h"

// OABI
//#define SYS_BASE 0x00900000

// EABI
#define SYS_BASE 0x0

#define SYS_exit (SYS_BASE + 1)
#define SYS_read (SYS_BASE + 3)
#define SYS_write (SYS_BASE + 4)
#define SYS_open (SYS_BASE + 5)
#define SYS_close (SYS_BASE + 6)
#define SYS_gettimeofday (SYS_BASE + 78)
#define SYS_clone (SYS_BASE + 120)
#define SYS_rt_sigreturn (SYS_BASE + 173)
#define SYS_rt_sigaction (SYS_BASE + 174)
#define SYS_sigaltstack (SYS_BASE + 186)
#define SYS_mmap2 (SYS_BASE + 192)
#define SYS_futex (SYS_BASE + 240)
#define SYS_exit_group (SYS_BASE + 248)
#define SYS_munmap (SYS_BASE + 91)
#define SYS_madvise (SYS_BASE + 220)
#define SYS_setitimer (SYS_BASE + 104)
#define SYS_mincore (SYS_BASE + 219)
#define SYS_gettid (SYS_BASE + 224)
#define SYS_tkill (SYS_BASE + 238)
#define SYS_sched_yield (SYS_BASE + 158)
#define SYS_select (SYS_BASE + 82)

#define ARM_BASE (SYS_BASE + 0x0f0000)
#define SYS_ARM_cacheflush (ARM_BASE + 2)

TEXT runtime·open(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	$SYS_open, R7
	SWI	$0
	RET

TEXT runtime·close(SB),7,$0
	MOVW	0(FP), R0
	MOVW	$SYS_close, R7
	SWI	$0
	RET

TEXT runtime·write(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	$SYS_write, R7
	SWI	$0
	RET

TEXT runtime·read(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	$SYS_read, R7
	SWI	$0
	RET

TEXT runtime·exit(SB),7,$-4
	MOVW	0(FP), R0
	MOVW	$SYS_exit_group, R7
	SWI	$0
	MOVW	$1234, R0
	MOVW	$1002, R1
	MOVW	R0, (R1)	// fail hard

TEXT runtime·exit1(SB),7,$-4
	MOVW	0(FP), R0
	MOVW	$SYS_exit, R7
	SWI	$0
	MOVW	$1234, R0
	MOVW	$1003, R1
	MOVW	R0, (R1)	// fail hard

TEXT	runtime·raisesigpipe(SB),7,$-4
	MOVW	$SYS_gettid, R7
	SWI	$0
	// arg 1 tid already in R0 from gettid
	MOVW	$13, R1	// arg 2 SIGPIPE
	MOVW	$SYS_tkill, R7
	SWI	$0
	RET

TEXT runtime·mmap(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	12(FP), R3
	MOVW	16(FP), R4
	MOVW	20(FP), R5
	MOVW	$SYS_mmap2, R7
	SWI	$0
	RET

TEXT runtime·munmap(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	$SYS_munmap, R7
	SWI	$0
	RET

TEXT runtime·madvise(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	$SYS_madvise, R7
	SWI	$0
	RET

TEXT runtime·setitimer(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	$SYS_setitimer, R7
	SWI	$0
	RET

TEXT runtime·mincore(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	$SYS_mincore, R7
	SWI	$0
	RET

TEXT time·now(SB), 7, $32
	MOVW	$8(R13), R0  // timeval
	MOVW	$0, R1  // zone
	MOVW	$SYS_gettimeofday, R7
	SWI	$0
	
	MOVW	8(R13), R0  // sec
	MOVW	12(R13), R2  // usec
	
	MOVW	R0, 0(FP)
	MOVW	$0, R1
	MOVW	R1, 4(FP)
	MOVW	$1000, R3
	MUL	R3, R2
	MOVW	R2, 8(FP)
	RET	

// int64 nanotime(void) so really
// void nanotime(int64 *nsec)
TEXT runtime·nanotime(SB),7,$32
	MOVW	$8(R13), R0  // timeval
	MOVW	$0, R1  // zone
	MOVW	$SYS_gettimeofday, R7
	SWI	$0
	
	MOVW	8(R13), R0  // sec
	MOVW	12(R13), R2  // usec
	
	MOVW	$1000000000, R3
	MULLU	R0, R3, (R1, R0)
	MOVW	$1000, R3
	MOVW	$0, R4
	MUL	R3, R2
	ADD.S	R2, R0
	ADC	R4, R1
	
	MOVW	0(FP), R3
	MOVW	R0, 0(R3)
	MOVW	R1, 4(R3)
	RET

// int32 futex(int32 *uaddr, int32 op, int32 val,
//	struct timespec *timeout, int32 *uaddr2, int32 val2);
TEXT runtime·futex(SB),7,$0
	MOVW	4(SP), R0
	MOVW	8(SP), R1
	MOVW	12(SP), R2
	MOVW	16(SP), R3
	MOVW	20(SP), R4
	MOVW	24(SP), R5
	MOVW	$SYS_futex, R7
	SWI	$0
	RET


// int32 clone(int32 flags, void *stack, M *m, G *g, void (*fn)(void));
TEXT runtime·clone(SB),7,$0
	MOVW	flags+0(FP), R0
	MOVW	stack+4(FP), R1
	MOVW	$0, R2	// parent tid ptr
	MOVW	$0, R3	// tls_val
	MOVW	$0, R4	// child tid ptr
	MOVW	$0, R5

	// Copy m, g, fn off parent stack for use by child.
	// TODO(kaib): figure out which registers are clobbered by clone and avoid stack copying
	MOVW	$-16(R1), R1
	MOVW	mm+8(FP), R6
	MOVW	R6, 0(R1)
	MOVW	gg+12(FP), R6
	MOVW	R6, 4(R1)
	MOVW	fn+16(FP), R6
	MOVW	R6, 8(R1)
	MOVW	$1234, R6
	MOVW	R6, 12(R1)

	MOVW	$SYS_clone, R7
	SWI	$0

	// In parent, return.
	CMP	$0, R0
	BEQ	2(PC)
	RET

	// Paranoia: check that SP is as we expect. Use R13 to avoid linker 'fixup'
	MOVW	12(R13), R0
	MOVW	$1234, R1
	CMP	R0, R1
	BEQ	2(PC)
	BL	runtime·abort(SB)

	MOVW	0(R13), m
	MOVW	4(R13), g

	// paranoia; check they are not nil
	MOVW	0(m), R0
	MOVW	0(g), R0

	BL	runtime·emptyfunc(SB)	// fault if stack check is wrong

	// Initialize m->procid to Linux tid
	MOVW	$SYS_gettid, R7
	SWI	$0
	MOVW	R0, m_procid(m)

	// Call fn
	MOVW	8(R13), R0
	MOVW	$16(R13), R13
	BL	(R0)

	MOVW	$0, R0
	MOVW	R0, 4(R13)
	BL	runtime·exit1(SB)

	// It shouldn't return
	MOVW	$1234, R0
	MOVW	$1005, R1
	MOVW	R0, (R1)


TEXT runtime·cacheflush(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	$0, R2
	MOVW	$SYS_ARM_cacheflush, R7
	SWI	$0
	RET

TEXT runtime·sigaltstack(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	$SYS_sigaltstack, R7
	SWI	$0
	RET

TEXT runtime·sigignore(SB),7,$0
	RET

TEXT runtime·sigtramp(SB),7,$24
	// save g
	MOVW	g, R3
	MOVW	g, 20(R13)

	// g = m->gsignal
	MOVW	m_gsignal(m), g

	// copy arguments for call to sighandler
	MOVW	R0, 4(R13)
	MOVW	R1, 8(R13)
	MOVW	R2, 12(R13)
	MOVW	R3, 16(R13)

	BL	runtime·sighandler(SB)

	// restore g
	MOVW	20(R13), g

	RET

TEXT runtime·rt_sigaction(SB),7,$0
	MOVW	0(FP), R0
	MOVW	4(FP), R1
	MOVW	8(FP), R2
	MOVW	12(FP), R3
	MOVW	$SYS_rt_sigaction, R7
	SWI	$0
	RET

TEXT runtime·sigreturn(SB),7,$0
	MOVW	$SYS_rt_sigreturn, R7
	SWI	$0
	RET

TEXT runtime·usleep(SB),7,$12
	MOVW	usec+0(FP), R0
	MOVW	R0, R1
	MOVW	$1000000, R2
	DIV	R1, R0
	MOD	R2, R0
	MOVW	R1, 4(SP)
	MOVW	R2, 8(SP)
	MOVW	$0, R0
	MOVW	$0, R1
	MOVW	$0, R2
	MOVW	$0, R3
	MOVW	$4(SP), R4
	MOVW	$SYS_select, R7
	SWI	$0
	RET

// Use kernel version instead of native armcas in ../../arm.s.
// See ../../../sync/atomic/asm_linux_arm.s for details.
TEXT cas<>(SB),7,$0
	MOVW	$0xffff0fc0, PC

TEXT runtime·cas(SB),7,$0
	MOVW	valptr+0(FP), R2
	MOVW	old+4(FP), R0
casagain:
	MOVW	new+8(FP), R1
	BL	cas<>(SB)
	BCC	cascheck
	MOVW $1, R0
	RET
cascheck:
	// Kernel lies; double-check.
	MOVW	valptr+0(FP), R2
	MOVW	old+4(FP), R0
	MOVW	0(R2), R3
	CMP	R0, R3
	BEQ	casagain
	MOVW $0, R0
	RET


TEXT runtime·casp(SB),7,$0
	B	runtime·cas(SB)

TEXT runtime·osyield(SB),7,$0
	MOVW	$SYS_sched_yield, R7
	SWI	$0
	RET
