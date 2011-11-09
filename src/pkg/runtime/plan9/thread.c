// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"
#include "os.h"
#include "arch.h"

int8 *goos = "plan9";

void
runtime·minit(void)
{
}

static int32
getproccount(void)
{
	int32 fd, i, n, ncpu;
	byte buf[2048];

	fd = runtime·open((byte*)"/dev/sysstat", OREAD);
	if(fd < 0)
		return 1;
	ncpu = 0;
	for(;;) {
		n = runtime·read(fd, buf, sizeof buf);
		if(n <= 0)
			break;
		for(i = 0; i < n; i++) {
			if(buf[i] == '\n')
				ncpu++;
		}
	}
	runtime·close(fd);
	return ncpu > 0 ? ncpu : 1;
}

void
runtime·osinit(void)
{
	runtime·ncpu = getproccount();
}

void
runtime·goenvs(void)
{
}

void
runtime·initsig(int32)
{
}

void
runtime·osyield(void)
{
	runtime·sleep(0);
}

void
runtime·usleep(uint32 µs)
{
	uint32 ms;

	ms = µs/1000;
	if(ms == 0)
		ms = 1;
	runtime·sleep(ms);
}

extern Tos *_tos;
void
runtime·exit(int32)
{
	int32 fd;
	uint8 buf[128];
	uint8 tmp[16];
	uint8 *p, *q;
	int32 pid;
	
	runtime·memclr(buf, sizeof buf);
	runtime·memclr(tmp, sizeof tmp);
	pid = _tos->pid;

	/* build path string /proc/pid/notepg */
	for(q=tmp; pid > 0;) {
		*q++ = '0' + (pid%10);
		pid = pid/10;
	}
	p = buf;
	runtime·memmove((void*)p, (void*)"/proc/", 6);
	p += 6;
	for(q--; q >= tmp;)
		*p++ = *q--;
	runtime·memmove((void*)p, (void*)"/notepg", 7);
	
	/* post interrupt note */
	fd = runtime·open(buf, OWRITE);
	runtime·write(fd, "interrupt", 9);
	runtime·exits(nil);
}

void
runtime·newosproc(M *m, G *g, void *stk, void (*fn)(void))
{
	m->tls[0] = m->id;	// so 386 asm can find it
	if(0){
		runtime·printf("newosproc stk=%p m=%p g=%p fn=%p rfork=%p id=%d/%d ostk=%p\n",
			stk, m, g, fn, runtime·rfork, m->id, m->tls[0], &m);
	}        
	
	if(runtime·rfork(RFPROC|RFMEM|RFNOWAIT, stk, m, g, fn) < 0)
		runtime·throw("newosproc: rfork failed");
}

uintptr
runtime·semacreate(void)
{
	return 1;
}

void
runtime·semasleep(void)
{
	while(runtime·plan9_semacquire(&m->waitsemacount, 1) < 0) {
		/* interrupted; try again */
	}
}

void
runtime·semawakeup(M *mp)
{
	runtime·plan9_semrelease(&mp->waitsemacount, 1);
}

void
os·sigpipe(void)
{
	runtime·throw("too many writes on closed pipe");
}

/*
 * placeholder - once notes are implemented,
 * a signal generating a panic must appear as
 * a call to this function for correct handling by
 * traceback.
 */
void
runtime·sigpanic(void)
{
}
