// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include "libcgo.h"

static void *threadentry(void*);

/* From what I've read 1MB is default for 32-bit Linux. 
   Allocation granularity on Windows is typically 64 KB. */
#define STACKSIZE (1*1024*1024)

static void
xinitcgo(void)
{
}

void (*initcgo)(void) = xinitcgo;

void
libcgo_sys_thread_start(ThreadStart *ts)
{
	ts->g->stackguard = STACKSIZE;
	_beginthread(threadentry, STACKSIZE, ts);
}

static void*
threadentry(void *v)
{
	ThreadStart ts;
	void *tls0;

	ts = *(ThreadStart*)v;
	free(v);

	ts.g->stackbase = (uintptr)&ts;

	/*
	 * libcgo_sys_thread_start set stackguard to stack size;
	 * change to actual guard pointer.
	 */
	ts.g->stackguard = (uintptr)&ts - ts.g->stackguard + 4096;

	/*
	 * Set specific keys in thread local storage.
	 */
	tls0 = (void*)LocalAlloc(LPTR, 32);
	asm volatile (
		"movl %0, %%fs:0x2c\n"	// MOVL tls0, 0x2c(FS)
		"movl %%fs:0x2c, %%eax\n"	// MOVL 0x2c(FS), tmp
		"movl %1, 0(%%eax)\n"	// MOVL g, 0(FS)
		"movl %2, 4(%%eax)\n"	// MOVL m, 4(FS)
		:: "r"(tls0), "r"(ts.g), "r"(ts.m) : "%eax"
	);
	
	crosscall_386(ts.fn);
	
	LocalFree(tls0);
	return nil;
}
