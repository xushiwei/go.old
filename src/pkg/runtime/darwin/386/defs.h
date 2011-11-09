// godefs -f -m32 defs.c

// MACHINE GENERATED - DO NOT EDIT.

// Constants
enum {
	PROT_NONE = 0,
	PROT_READ = 0x1,
	PROT_WRITE = 0x2,
	PROT_EXEC = 0x4,
	MAP_ANON = 0x1000,
	MAP_PRIVATE = 0x2,
	MAP_FIXED = 0x10,
	MACH_MSG_TYPE_MOVE_RECEIVE = 0x10,
	MACH_MSG_TYPE_MOVE_SEND = 0x11,
	MACH_MSG_TYPE_MOVE_SEND_ONCE = 0x12,
	MACH_MSG_TYPE_COPY_SEND = 0x13,
	MACH_MSG_TYPE_MAKE_SEND = 0x14,
	MACH_MSG_TYPE_MAKE_SEND_ONCE = 0x15,
	MACH_MSG_TYPE_COPY_RECEIVE = 0x16,
	MACH_MSG_PORT_DESCRIPTOR = 0,
	MACH_MSG_OOL_DESCRIPTOR = 0x1,
	MACH_MSG_OOL_PORTS_DESCRIPTOR = 0x2,
	MACH_MSG_OOL_VOLATILE_DESCRIPTOR = 0x3,
	MACH_MSGH_BITS_COMPLEX = 0x80000000,
	MACH_SEND_MSG = 0x1,
	MACH_RCV_MSG = 0x2,
	MACH_RCV_LARGE = 0x4,
	MACH_SEND_TIMEOUT = 0x10,
	MACH_SEND_INTERRUPT = 0x40,
	MACH_SEND_CANCEL = 0x80,
	MACH_SEND_ALWAYS = 0x10000,
	MACH_SEND_TRAILER = 0x20000,
	MACH_RCV_TIMEOUT = 0x100,
	MACH_RCV_NOTIFY = 0x200,
	MACH_RCV_INTERRUPT = 0x400,
	MACH_RCV_OVERWRITE = 0x1000,
	NDR_PROTOCOL_2_0 = 0,
	NDR_INT_BIG_ENDIAN = 0,
	NDR_INT_LITTLE_ENDIAN = 0x1,
	NDR_FLOAT_IEEE = 0,
	NDR_CHAR_ASCII = 0,
	SA_SIGINFO = 0x40,
	SA_RESTART = 0x2,
	SA_ONSTACK = 0x1,
	SA_USERTRAMP = 0x100,
	SA_64REGSET = 0x200,
	SIGHUP = 0x1,
	SIGINT = 0x2,
	SIGQUIT = 0x3,
	SIGILL = 0x4,
	SIGTRAP = 0x5,
	SIGABRT = 0x6,
	SIGEMT = 0x7,
	SIGFPE = 0x8,
	SIGKILL = 0x9,
	SIGBUS = 0xa,
	SIGSEGV = 0xb,
	SIGSYS = 0xc,
	SIGPIPE = 0xd,
	SIGALRM = 0xe,
	SIGTERM = 0xf,
	SIGURG = 0x10,
	SIGSTOP = 0x11,
	SIGTSTP = 0x12,
	SIGCONT = 0x13,
	SIGCHLD = 0x14,
	SIGTTIN = 0x15,
	SIGTTOU = 0x16,
	SIGIO = 0x17,
	SIGXCPU = 0x18,
	SIGXFSZ = 0x19,
	SIGVTALRM = 0x1a,
	SIGPROF = 0x1b,
	SIGWINCH = 0x1c,
	SIGINFO = 0x1d,
	SIGUSR1 = 0x1e,
	SIGUSR2 = 0x1f,
	FPE_INTDIV = 0x7,
	FPE_INTOVF = 0x8,
	FPE_FLTDIV = 0x1,
	FPE_FLTOVF = 0x2,
	FPE_FLTUND = 0x3,
	FPE_FLTRES = 0x4,
	FPE_FLTINV = 0x5,
	FPE_FLTSUB = 0x6,
	BUS_ADRALN = 0x1,
	BUS_ADRERR = 0x2,
	BUS_OBJERR = 0x3,
	SEGV_MAPERR = 0x1,
	SEGV_ACCERR = 0x2,
	ITIMER_REAL = 0,
	ITIMER_VIRTUAL = 0x1,
	ITIMER_PROF = 0x2,
};

// Types
#pragma pack on

typedef struct MachBody MachBody;
struct MachBody {
	uint32 msgh_descriptor_count;
};

typedef struct MachHeader MachHeader;
struct MachHeader {
	uint32 msgh_bits;
	uint32 msgh_size;
	uint32 msgh_remote_port;
	uint32 msgh_local_port;
	uint32 msgh_reserved;
	int32 msgh_id;
};

typedef struct MachNDR MachNDR;
struct MachNDR {
	uint8 mig_vers;
	uint8 if_vers;
	uint8 reserved1;
	uint8 mig_encoding;
	uint8 int_rep;
	uint8 char_rep;
	uint8 float_rep;
	uint8 reserved2;
};

typedef struct MachPort MachPort;
struct MachPort {
	uint32 name;
	uint32 pad1;
	uint16 pad2;
	uint8 disposition;
	uint8 type;
};

typedef struct StackT StackT;
struct StackT {
	void *ss_sp;
	uint32 ss_size;
	int32 ss_flags;
};

typedef union Sighandler Sighandler;
union Sighandler {
	uint32 __sa_handler;
	uint32 __sa_sigaction;
};

typedef struct Sigaction Sigaction;
struct Sigaction {
	Sighandler __sigaction_u;
	uint32 sa_tramp;
	uint32 sa_mask;
	int32 sa_flags;
};

typedef union Sigval Sigval;
union Sigval {
	int32 sival_int;
	void *sival_ptr;
};

typedef struct Siginfo Siginfo;
struct Siginfo {
	int32 si_signo;
	int32 si_errno;
	int32 si_code;
	int32 si_pid;
	uint32 si_uid;
	int32 si_status;
	void *si_addr;
	Sigval si_value;
	int32 si_band;
	uint32 __pad[7];
};

typedef struct Timeval Timeval;
struct Timeval {
	int32 tv_sec;
	int32 tv_usec;
};

typedef struct Itimerval Itimerval;
struct Itimerval {
	Timeval it_interval;
	Timeval it_value;
};

typedef struct FPControl FPControl;
struct FPControl {
	byte pad_godefs_0[2];
};

typedef struct FPStatus FPStatus;
struct FPStatus {
	byte pad_godefs_0[2];
};

typedef struct RegMMST RegMMST;
struct RegMMST {
	int8 mmst_reg[10];
	int8 mmst_rsrv[6];
};

typedef struct RegXMM RegXMM;
struct RegXMM {
	int8 xmm_reg[16];
};

typedef struct Regs Regs;
struct Regs {
	uint32 eax;
	uint32 ebx;
	uint32 ecx;
	uint32 edx;
	uint32 edi;
	uint32 esi;
	uint32 ebp;
	uint32 esp;
	uint32 ss;
	uint32 eflags;
	uint32 eip;
	uint32 cs;
	uint32 ds;
	uint32 es;
	uint32 fs;
	uint32 gs;
};

typedef struct FloatState FloatState;
struct FloatState {
	uint64 fpu_reserved;
	FPControl fpu_fcw;
	FPStatus fpu_fsw;
	uint8 fpu_ftw;
	uint8 fpu_rsrv1;
	uint16 fpu_fop;
	uint32 fpu_ip;
	uint16 fpu_cs;
	uint16 fpu_rsrv2;
	uint32 fpu_dp;
	uint16 fpu_ds;
	uint16 fpu_rsrv3;
	uint32 fpu_mxcsr;
	uint32 fpu_mxcsrmask;
	RegMMST fpu_stmm0;
	RegMMST fpu_stmm1;
	RegMMST fpu_stmm2;
	RegMMST fpu_stmm3;
	RegMMST fpu_stmm4;
	RegMMST fpu_stmm5;
	RegMMST fpu_stmm6;
	RegMMST fpu_stmm7;
	RegXMM fpu_xmm0;
	RegXMM fpu_xmm1;
	RegXMM fpu_xmm2;
	RegXMM fpu_xmm3;
	RegXMM fpu_xmm4;
	RegXMM fpu_xmm5;
	RegXMM fpu_xmm6;
	RegXMM fpu_xmm7;
	int8 fpu_rsrv4[224];
	int32 fpu_reserved1;
};

typedef struct ExceptionState ExceptionState;
struct ExceptionState {
	uint32 trapno;
	uint32 err;
	uint32 faultvaddr;
};

typedef struct Mcontext Mcontext;
struct Mcontext {
	ExceptionState es;
	Regs ss;
	FloatState fs;
};

typedef struct Ucontext Ucontext;
struct Ucontext {
	int32 uc_onstack;
	uint32 uc_sigmask;
	StackT uc_stack;
	uint32 uc_link;
	uint32 uc_mcsize;
	Mcontext *uc_mcontext;
};
#pragma pack off
