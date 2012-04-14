// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"
#include "defs_GOOS_GOARCH.h"
#include "signals_GOOS.h"
#include "os_GOOS.h"

extern void runtime·sigtramp(void);

typedef struct sigaction {
	union {
		void    (*__sa_handler)(int32);
		void    (*__sa_sigaction)(int32, Siginfo*, void *);
	} __sigaction_u;		/* signal handler */
	uint32	sa_mask;		/* signal mask to apply */
	int32	sa_flags;		/* see signal options below */
} Sigaction;

void
runtime·dumpregs(Sigcontext *r)
{
	runtime·printf("rax     %X\n", r->sc_rax);
	runtime·printf("rbx     %X\n", r->sc_rbx);
	runtime·printf("rcx     %X\n", r->sc_rcx);
	runtime·printf("rdx     %X\n", r->sc_rdx);
	runtime·printf("rdi     %X\n", r->sc_rdi);
	runtime·printf("rsi     %X\n", r->sc_rsi);
	runtime·printf("rbp     %X\n", r->sc_rbp);
	runtime·printf("rsp     %X\n", r->sc_rsp);
	runtime·printf("r8      %X\n", r->sc_r8);
	runtime·printf("r9      %X\n", r->sc_r9);
	runtime·printf("r10     %X\n", r->sc_r10);
	runtime·printf("r11     %X\n", r->sc_r11);
	runtime·printf("r12     %X\n", r->sc_r12);
	runtime·printf("r13     %X\n", r->sc_r13);
	runtime·printf("r14     %X\n", r->sc_r14);
	runtime·printf("r15     %X\n", r->sc_r15);
	runtime·printf("rip     %X\n", r->sc_rip);
	runtime·printf("rflags  %X\n", r->sc_rflags);
	runtime·printf("cs      %X\n", r->sc_cs);
	runtime·printf("fs      %X\n", r->sc_fs);
	runtime·printf("gs      %X\n", r->sc_gs);
}

void
runtime·sighandler(int32 sig, Siginfo *info, void *context, G *gp)
{
	Sigcontext *r = context;
	uintptr *sp;
	SigTab *t;

	if(sig == SIGPROF) {
		runtime·sigprof((uint8*)r->sc_rip,
			(uint8*)r->sc_rsp, nil, gp);
		return;
	}

	t = &runtime·sigtab[sig];
	if(info->si_code != SI_USER && (t->flags & SigPanic)) {
		if(gp == nil)
			goto Throw;
		// Make it look like a call to the signal func.
		// Have to pass arguments out of band since
		// augmenting the stack frame would break
		// the unwinding code.
		gp->sig = sig;
		gp->sigcode0 = info->si_code;
		gp->sigcode1 = *(uintptr*)((byte*)info + 16); /* si_addr */
		gp->sigpc = r->sc_rip;

		// Only push runtime·sigpanic if r->mc_rip != 0.
		// If r->mc_rip == 0, probably panicked because of a
		// call to a nil func.  Not pushing that onto sp will
		// make the trace look like a call to runtime·sigpanic instead.
		// (Otherwise the trace will end at runtime·sigpanic and we
		// won't get to see who faulted.)
		if(r->sc_rip != 0) {
			sp = (uintptr*)r->sc_rsp;
			*--sp = r->sc_rip;
			r->sc_rsp = (uintptr)sp;
		}
		r->sc_rip = (uintptr)runtime·sigpanic;
		return;
	}

	if(info->si_code == SI_USER || (t->flags & SigNotify))
		if(runtime·sigsend(sig))
			return;
	if(t->flags & SigKill)
		runtime·exit(2);
	if(!(t->flags & SigThrow))
		return;

Throw:
	runtime·startpanic();

	if(sig < 0 || sig >= NSIG)
		runtime·printf("Signal %d\n", sig);
	else
		runtime·printf("%s\n", runtime·sigtab[sig].name);

	runtime·printf("PC=%X\n", r->sc_rip);
	runtime·printf("\n");

	if(runtime·gotraceback()){
		runtime·traceback((void*)r->sc_rip, (void*)r->sc_rsp, 0, gp);
		runtime·tracebackothers(gp);
		runtime·dumpregs(r);
	}

	runtime·exit(2);
}

void
runtime·signalstack(byte *p, int32 n)
{
	Sigaltstack st;

	st.ss_sp = (int8*)p;
	st.ss_size = n;
	st.ss_flags = 0;
	runtime·sigaltstack(&st, nil);
}

void
runtime·setsig(int32 i, void (*fn)(int32, Siginfo*, void*, G*), bool restart)
{
	Sigaction sa;

	runtime·memclr((byte*)&sa, sizeof sa);
	sa.sa_flags = SA_SIGINFO|SA_ONSTACK;
	if(restart)
		sa.sa_flags |= SA_RESTART;
	sa.sa_mask = ~0ULL;
	if (fn == runtime·sighandler)
		fn = (void*)runtime·sigtramp;
	sa.__sigaction_u.__sa_sigaction = (void*)fn;
	runtime·sigaction(i, &sa, nil);
}
