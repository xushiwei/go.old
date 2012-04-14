// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"
#include "stack.h"

enum {
	maxround = sizeof(uintptr),
};

uint32	runtime·panicking;
void	(*runtime·destroylock)(Lock*);

/*
 * We assume that all architectures turn faults and the like
 * into apparent calls to runtime.sigpanic.  If we see a "call"
 * to runtime.sigpanic, we do not back up the PC to find the
 * line number of the CALL instruction, because there is no CALL.
 */
void	runtime·sigpanic(void);

int32
runtime·gotraceback(void)
{
	byte *p;

	p = runtime·getenv("GOTRACEBACK");
	if(p == nil || p[0] == '\0')
		return 1;	// default is on
	return runtime·atoi(p);
}

static Lock paniclk;

void
runtime·startpanic(void)
{
	if(m->dying) {
		runtime·printf("panic during panic\n");
		runtime·exit(3);
	}
	m->dying = 1;
	runtime·xadd(&runtime·panicking, 1);
	runtime·lock(&paniclk);
}

void
runtime·dopanic(int32 unused)
{
	static bool didothers;

	if(g->sig != 0)
		runtime·printf("[signal %x code=%p addr=%p pc=%p]\n",
			g->sig, g->sigcode0, g->sigcode1, g->sigpc);

	if(runtime·gotraceback()){
		if(g != m->g0) {
			runtime·printf("\n");
			runtime·goroutineheader(g);
			runtime·traceback(runtime·getcallerpc(&unused), runtime·getcallersp(&unused), 0, g);
		}
		if(!didothers) {
			didothers = true;
			runtime·tracebackothers(g);
		}
	}
	runtime·unlock(&paniclk);
	if(runtime·xadd(&runtime·panicking, -1) != 0) {
		// Some other m is panicking too.
		// Let it print what it needs to print.
		// Wait forever without chewing up cpu.
		// It will exit when it's done.
		static Lock deadlock;
		runtime·lock(&deadlock);
		runtime·lock(&deadlock);
	}

	runtime·exit(2);
}

void
runtime·panicindex(void)
{
	runtime·panicstring("index out of range");
}

void
runtime·panicslice(void)
{
	runtime·panicstring("slice bounds out of range");
}

void
runtime·throwreturn(void)
{
	// can only happen if compiler is broken
	runtime·throw("no return at end of a typed function - compiler is broken");
}

void
runtime·throwinit(void)
{
	// can only happen with linker skew
	runtime·throw("recursive call during initialization - linker skew");
}

void
runtime·throw(int8 *s)
{
	runtime·startpanic();
	runtime·printf("throw: %s\n", s);
	runtime·dopanic(0);
	*(int32*)0 = 0;	// not reached
	runtime·exit(1);	// even more not reached
}

void
runtime·panicstring(int8 *s)
{
	Eface err;

	if(m->gcing) {
		runtime·printf("panic: %s\n", s);
		runtime·throw("panic during gc");
	}
	runtime·newErrorString(runtime·gostringnocopy((byte*)s), &err);
	runtime·panic(err);
}

int32
runtime·mcmp(byte *s1, byte *s2, uint32 n)
{
	uint32 i;
	byte c1, c2;

	for(i=0; i<n; i++) {
		c1 = s1[i];
		c2 = s2[i];
		if(c1 < c2)
			return -1;
		if(c1 > c2)
			return +1;
	}
	return 0;
}


byte*
runtime·mchr(byte *p, byte c, byte *ep)
{
	for(; p < ep; p++)
		if(*p == c)
			return p;
	return nil;
}

uint32
runtime·rnd(uint32 n, uint32 m)
{
	uint32 r;

	if(m > maxround)
		m = maxround;
	r = n % m;
	if(r)
		n += m-r;
	return n;
}

static int32	argc;
static uint8**	argv;

Slice os·Args;
Slice syscall·envs;

void
runtime·args(int32 c, uint8 **v)
{
	argc = c;
	argv = v;
}

int32 runtime·isplan9;
int32 runtime·iswindows;

void
runtime·goargs(void)
{
	String *s;
	int32 i;

	// for windows implementation see "os" package
	if(Windows)
		return;

	s = runtime·malloc(argc*sizeof s[0]);
	for(i=0; i<argc; i++)
		s[i] = runtime·gostringnocopy(argv[i]);
	os·Args.array = (byte*)s;
	os·Args.len = argc;
	os·Args.cap = argc;
}

void
runtime·goenvs_unix(void)
{
	String *s;
	int32 i, n;

	for(n=0; argv[argc+1+n] != 0; n++)
		;

	s = runtime·malloc(n*sizeof s[0]);
	for(i=0; i<n; i++)
		s[i] = runtime·gostringnocopy(argv[argc+1+i]);
	syscall·envs.array = (byte*)s;
	syscall·envs.len = n;
	syscall·envs.cap = n;
}

byte*
runtime·getenv(int8 *s)
{
	int32 i, j, len;
	byte *v, *bs;
	String* envv;
	int32 envc;

	bs = (byte*)s;
	len = runtime·findnull(bs);
	envv = (String*)syscall·envs.array;
	envc = syscall·envs.len;
	for(i=0; i<envc; i++){
		if(envv[i].len <= len)
			continue;
		v = envv[i].str;
		for(j=0; j<len; j++)
			if(bs[j] != v[j])
				goto nomatch;
		if(v[len] != '=')
			goto nomatch;
		return v+len+1;
	nomatch:;
	}
	return nil;
}

void
runtime·getgoroot(String out)
{
	byte *p;

	p = runtime·getenv("GOROOT");
	out = runtime·gostringnocopy(p);
	FLUSH(&out);
}

int32
runtime·atoi(byte *p)
{
	int32 n;

	n = 0;
	while('0' <= *p && *p <= '9')
		n = n*10 + *p++ - '0';
	return n;
}

void
runtime·check(void)
{
	int8 a;
	uint8 b;
	int16 c;
	uint16 d;
	int32 e;
	uint32 f;
	int64 g;
	uint64 h;
	float32 i, i1;
	float64 j, j1;
	void* k;
	uint16* l;
	struct x1 {
		byte x;
	};
	struct y1 {
		struct x1 x1;
		byte y;
	};

	if(sizeof(a) != 1) runtime·throw("bad a");
	if(sizeof(b) != 1) runtime·throw("bad b");
	if(sizeof(c) != 2) runtime·throw("bad c");
	if(sizeof(d) != 2) runtime·throw("bad d");
	if(sizeof(e) != 4) runtime·throw("bad e");
	if(sizeof(f) != 4) runtime·throw("bad f");
	if(sizeof(g) != 8) runtime·throw("bad g");
	if(sizeof(h) != 8) runtime·throw("bad h");
	if(sizeof(i) != 4) runtime·throw("bad i");
	if(sizeof(j) != 8) runtime·throw("bad j");
	if(sizeof(k) != sizeof(uintptr)) runtime·throw("bad k");
	if(sizeof(l) != sizeof(uintptr)) runtime·throw("bad l");
	if(sizeof(struct x1) != 1) runtime·throw("bad sizeof x1");
	if(offsetof(struct y1, y) != 1) runtime·throw("bad offsetof y1.y");
	if(sizeof(struct y1) != 2) runtime·throw("bad sizeof y1");

	uint32 z;
	z = 1;
	if(!runtime·cas(&z, 1, 2))
		runtime·throw("cas1");
	if(z != 2)
		runtime·throw("cas2");

	z = 4;
	if(runtime·cas(&z, 5, 6))
		runtime·throw("cas3");
	if(z != 4)
		runtime·throw("cas4");

	*(uint64*)&j = ~0ULL;
	if(j == j)
		runtime·throw("float64nan");
	if(!(j != j))
		runtime·throw("float64nan1");

	*(uint64*)&j1 = ~1ULL;
	if(j == j1)
		runtime·throw("float64nan2");
	if(!(j != j1))
		runtime·throw("float64nan3");

	*(uint32*)&i = ~0UL;
	if(i == i)
		runtime·throw("float32nan");
	if(!(i != i))
		runtime·throw("float32nan1");

	*(uint32*)&i1 = ~1UL;
	if(i == i1)
		runtime·throw("float32nan2");
	if(!(i != i1))
		runtime·throw("float32nan3");
}

void
runtime·Caller(int32 skip, uintptr retpc, String retfile, int32 retline, bool retbool)
{
	Func *f, *g;
	uintptr pc;
	uintptr rpc[2];

	/*
	 * Ask for two PCs: the one we were asked for
	 * and what it called, so that we can see if it
	 * "called" sigpanic.
	 */
	retpc = 0;
	if(runtime·callers(1+skip-1, rpc, 2) < 2) {
		retfile = runtime·emptystring;
		retline = 0;
		retbool = false;
	} else if((f = runtime·findfunc(rpc[1])) == nil) {
		retfile = runtime·emptystring;
		retline = 0;
		retbool = true;  // have retpc at least
	} else {
		retpc = rpc[1];
		retfile = f->src;
		pc = retpc;
		g = runtime·findfunc(rpc[0]);
		if(pc > f->entry && (g == nil || g->entry != (uintptr)runtime·sigpanic))
			pc--;
		retline = runtime·funcline(f, pc);
		retbool = true;
	}
	FLUSH(&retpc);
	FLUSH(&retfile);
	FLUSH(&retline);
	FLUSH(&retbool);
}

void
runtime·Callers(int32 skip, Slice pc, int32 retn)
{
	// runtime.callers uses pc.array==nil as a signal
	// to print a stack trace.  Pick off 0-length pc here
	// so that we don't let a nil pc slice get to it.
	if(pc.len == 0)
		retn = 0;
	else
		retn = runtime·callers(skip, (uintptr*)pc.array, pc.len);
	FLUSH(&retn);
}

void
runtime·FuncForPC(uintptr pc, void *retf)
{
	retf = runtime·findfunc(pc);
	FLUSH(&retf);
}

uint32
runtime·fastrand1(void)
{
	uint32 x;

	x = m->fastrand;
	x += x;
	if(x & 0x80000000L)
		x ^= 0x88888eefUL;
	m->fastrand = x;
	return x;
}
