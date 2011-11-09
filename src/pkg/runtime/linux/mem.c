// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"
#include "arch.h"
#include "defs.h"
#include "os.h"
#include "malloc.h"

enum
{
	ENOMEM = 12,
	_PAGE_SIZE = 4096,
};

static int32
addrspace_free(void *v, uintptr n)
{
	int32 errval;
	uintptr chunk;
	uintptr off;
	static byte vec[4096];

	for(off = 0; off < n; off += chunk) {
		chunk = _PAGE_SIZE * sizeof vec;
		if(chunk > (n - off))
			chunk = n - off;
		errval = runtime·mincore((int8*)v + off, chunk, vec);
		// errval is 0 if success, or -(error_code) if error.
		if (errval == 0 || errval != -ENOMEM)
			return 0;
	}
	return 1;
}


void*
runtime·SysAlloc(uintptr n)
{
	void *p;

	mstats.sys += n;
	p = runtime·mmap(nil, n, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_ANON|MAP_PRIVATE, -1, 0);
	if(p < (void*)4096) {
		if(p == (void*)EACCES) {
			runtime·printf("runtime: mmap: access denied\n");
			runtime·printf("if you're running SELinux, enable execmem for this process.\n");
			runtime·exit(2);
		}
		return nil;
	}
	return p;
}

void
runtime·SysUnused(void *v, uintptr n)
{
	USED(v);
	USED(n);
	// TODO(rsc): call madvise MADV_DONTNEED
}

void
runtime·SysFree(void *v, uintptr n)
{
	mstats.sys -= n;
	runtime·munmap(v, n);
}

void*
runtime·SysReserve(void *v, uintptr n)
{
	void *p;

	// On 64-bit, people with ulimit -v set complain if we reserve too
	// much address space.  Instead, assume that the reservation is okay
	// and check the assumption in SysMap.
	if(sizeof(void*) == 8)
		return v;
	
	p = runtime·mmap(v, n, PROT_NONE, MAP_ANON|MAP_PRIVATE, -1, 0);
	if((uintptr)p < 4096 || -(uintptr)p < 4096) {
		return nil;
	}
	return p;
}

void
runtime·SysMap(void *v, uintptr n)
{
	void *p;
	
	mstats.sys += n;

	// On 64-bit, we don't actually have v reserved, so tread carefully.
	if(sizeof(void*) == 8) {
		p = runtime·mmap(v, n, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_ANON|MAP_PRIVATE, -1, 0);
		if(p != v && addrspace_free(v, n)) {
			// On some systems, mmap ignores v without
			// MAP_FIXED, so retry if the address space is free.
			if(p > (void*)4096) {
				runtime·munmap(p, n);
			}
			p = runtime·mmap(v, n, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_ANON|MAP_FIXED|MAP_PRIVATE, -1, 0);
		}
		if(p == (void*)ENOMEM)
			runtime·throw("runtime: out of memory");
		if(p != v) {
			runtime·printf("runtime: address space conflict: map(%p) = %p\n", v, p);
			runtime·throw("runtime: address space conflict");
		}
		return;
	}

	p = runtime·mmap(v, n, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_ANON|MAP_FIXED|MAP_PRIVATE, -1, 0);
	if(p == (void*)ENOMEM)
		runtime·throw("runtime: out of memory");
	if(p != v)
		runtime·throw("runtime: cannot map pages in arena address space");
}
