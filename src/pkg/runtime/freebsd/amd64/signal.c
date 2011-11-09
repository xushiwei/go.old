// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"
#include "defs.h"
#include "signals.h"
#include "os.h"

extern void runtime·sigtramp(void);

typedef struct sigaction {
	union {
		void    (*__sa_handler)(int32);
		void    (*__sa_sigaction)(int32, Siginfo*, void *);
	} __sigaction_u;		/* signal handler */
	int32	sa_flags;		/* see signal options below */
	int64	sa_mask;		/* signal mask to apply */
} Sigaction;

void
runtime·dumpregs(Mcontext *r)
{
	runtime·printf("rax     %X\n", r->mc_rax);
	runtime·printf("rbx     %X\n", r->mc_rbx);
	runtime·printf("rcx     %X\n", r->mc_rcx);
	runtime·printf("rdx     %X\n", r->mc_rdx);
	runtime·printf("rdi     %X\n", r->mc_rdi);
	runtime·printf("rsi     %X\n", r->mc_rsi);
	runtime·printf("rbp     %X\n", r->mc_rbp);
	runtime·printf("rsp     %X\n", r->mc_rsp);
	runtime·printf("r8      %X\n", r->mc_r8 );
	runtime·printf("r9      %X\n", r->mc_r9 );
	runtime·printf("r10     %X\n", r->mc_r10);
	runtime·printf("r11     %X\n", r->mc_r11);
	runtime·printf("r12     %X\n", r->mc_r12);
	runtime·printf("r13     %X\n", r->mc_r13);
	runtime·printf("r14     %X\n", r->mc_r14);
	runtime·printf("r15     %X\n", r->mc_r15);
	runtime·printf("rip     %X\n", r->mc_rip);
	runtime·printf("rflags  %X\n", r->mc_flags);
	runtime·printf("cs      %X\n", r->mc_cs);
	runtime·printf("fs      %X\n", r->mc_fs);
	runtime·printf("gs      %X\n", r->mc_gs);
}

String
runtime·signame(int32 sig)
{
	if(sig < 0 || sig >= NSIG)
		return runtime·emptystring;
	return runtime·gostringnocopy((byte*)runtime·sigtab[sig].name);
}

void
runtime·sighandler(int32 sig, Siginfo *info, void *context, G *gp)
{
	Ucontext *uc;
	Mcontext *r;
	uintptr *sp;

	uc = context;
	r = &uc->uc_mcontext;

	if(sig == SIGPROF) {
		runtime·sigprof((uint8*)r->mc_rip, (uint8*)r->mc_rsp, nil, gp);
		return;
	}

	if(gp != nil && (runtime·sigtab[sig].flags & SigPanic)) {
		// Make it look like a call to the signal func.
		// Have to pass arguments out of band since
		// augmenting the stack frame would break
		// the unwinding code.
		gp->sig = sig;
		gp->sigcode0 = info->si_code;
		gp->sigcode1 = (uintptr)info->si_addr;
		gp->sigpc = r->mc_rip;

		// Only push runtime·sigpanic if r->mc_rip != 0.
		// If r->mc_rip == 0, probably panicked because of a
		// call to a nil func.  Not pushing that onto sp will
		// make the trace look like a call to runtime·sigpanic instead.
		// (Otherwise the trace will end at runtime·sigpanic and we
		// won't get to see who faulted.)
		if(r->mc_rip != 0) {
			sp = (uintptr*)r->mc_rsp;
			*--sp = r->mc_rip;
			r->mc_rsp = (uintptr)sp;
		}
		r->mc_rip = (uintptr)runtime·sigpanic;
		return;
	}

	if(runtime·sigtab[sig].flags & SigQueue) {
		if(runtime·sigsend(sig) || (runtime·sigtab[sig].flags & SigIgnore))
			return;
		runtime·exit(2);	// SIGINT, SIGTERM, etc
	}

	if(runtime·panicking)	// traceback already printed
		runtime·exit(2);
	runtime·panicking = 1;

	if(sig < 0 || sig >= NSIG)
		runtime·printf("Signal %d\n", sig);
	else
		runtime·printf("%s\n", runtime·sigtab[sig].name);

	runtime·printf("PC=%X\n", r->mc_rip);
	runtime·printf("\n");

	if(runtime·gotraceback()){
		runtime·traceback((void*)r->mc_rip, (void*)r->mc_rsp, 0, gp);
		runtime·tracebackothers(gp);
		runtime·dumpregs(r);
	}

	runtime·exit(2);
}

// Called from kernel on signal stack, so no stack split.
#pragma textflag 7
void
runtime·sigignore(void)
{
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

static void
sigaction(int32 i, void (*fn)(int32, Siginfo*, void*, G*), bool restart)
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

void
runtime·initsig(int32 queue)
{
	int32 i;
	void *fn;

	runtime·siginit();

	for(i = 0; i<NSIG; i++) {
		if(runtime·sigtab[i].flags) {
			if((runtime·sigtab[i].flags & SigQueue) != queue)
				continue;
			if(runtime·sigtab[i].flags & (SigCatch | SigQueue))
				fn = runtime·sighandler;
			else
				fn = runtime·sigignore;
			sigaction(i, fn, (runtime·sigtab[i].flags & SigRestart) != 0);
		}
	}
}

void
runtime·resetcpuprofiler(int32 hz)
{
	Itimerval it;
	
	runtime·memclr((byte*)&it, sizeof it);
	if(hz == 0) {
		runtime·setitimer(ITIMER_PROF, &it, nil);
		sigaction(SIGPROF, SIG_IGN, true);
	} else {
		sigaction(SIGPROF, runtime·sighandler, true);
		it.it_interval.tv_sec = 0;
		it.it_interval.tv_usec = 1000000 / hz;
		it.it_value = it.it_interval;
		runtime·setitimer(ITIMER_PROF, &it, nil);
	}
	m->profilehz = hz;
}

void
os·sigpipe(void)
{
	sigaction(SIGPIPE, SIG_DFL, false);
	runtime·raisesigpipe();
}
