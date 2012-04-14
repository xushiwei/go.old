// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#undef	EXTERN
#define	EXTERN
#include <u.h>
#include <libc.h>
#include "gg.h"
#include "opt.h"

void
defframe(Prog *ptxt)
{
	// fill in argument size
	ptxt->to.offset2 = rnd(curfn->type->argwid, widthptr);

	// fill in final stack size
	if(stksize > maxstksize)
		maxstksize = stksize;
	ptxt->to.offset = rnd(maxstksize+maxarg, widthptr);
	maxstksize = 0;
}

// Sweep the prog list to mark any used nodes.
void
markautoused(Prog* p)
{
	for (; p; p = p->link) {
		if (p->from.type == D_AUTO && p->from.node)
			p->from.node->used = 1;

		if (p->to.type == D_AUTO && p->to.node)
			p->to.node->used = 1;
	}
}

// Fixup instructions after compactframe has moved all autos around.
void
fixautoused(Prog* p)
{
	for (; p; p = p->link) {
		if (p->from.type == D_AUTO && p->from.node)
			p->from.offset += p->from.node->stkdelta;

		if (p->to.type == D_AUTO && p->to.node)
			p->to.offset += p->to.node->stkdelta;
	}
}

void
clearfat(Node *nl)
{
	uint32 w, c, q;
	Node n1;

	/* clear a fat object */
	if(debug['g'])
		dump("\nclearfat", nl);

	w = nl->type->width;
	c = w % 4;	// bytes
	q = w / 4;	// quads

	gconreg(AMOVL, 0, D_AX);
	nodreg(&n1, types[tptr], D_DI);
	agen(nl, &n1);

	if(q >= 4) {
		gconreg(AMOVL, q, D_CX);
		gins(AREP, N, N);	// repeat
		gins(ASTOSL, N, N);	// STOL AL,*(DI)+
	} else
	while(q > 0) {
		gins(ASTOSL, N, N);	// STOL AL,*(DI)+
		q--;
	}

	if(c >= 4) {
		gconreg(AMOVL, c, D_CX);
		gins(AREP, N, N);	// repeat
		gins(ASTOSB, N, N);	// STOB AL,*(DI)+
	} else
	while(c > 0) {
		gins(ASTOSB, N, N);	// STOB AL,*(DI)+
		c--;
	}
}

/*
 * generate:
 *	call f
 *	proc=0	normal call
 *	proc=1	goroutine run in new proc
 *	proc=2	defer call save away stack
 */
void
ginscall(Node *f, int proc)
{
	Prog *p;
	Node reg, con;

	switch(proc) {
	default:
		fatal("ginscall: bad proc %d", proc);
		break;

	case 0:	// normal call
		p = gins(ACALL, N, f);
		afunclit(&p->to);
		break;

	case 1:	// call in new proc (go)
	case 2:	// deferred call (defer)
		nodreg(&reg, types[TINT32], D_CX);
		gins(APUSHL, f, N);
		nodconst(&con, types[TINT32], argsize(f->type));
		gins(APUSHL, &con, N);
		if(proc == 1)
			ginscall(newproc, 0);
		else
			ginscall(deferproc, 0);
		gins(APOPL, N, &reg);
		gins(APOPL, N, &reg);
		if(proc == 2) {
			nodreg(&reg, types[TINT64], D_AX);
			gins(ATESTL, &reg, &reg);
			patch(gbranch(AJNE, T), retpc);
		}
		break;
	}
}

/*
 * n is call to interface method.
 * generate res = n.
 */
void
cgen_callinter(Node *n, Node *res, int proc)
{
	Node *i, *f;
	Node tmpi, nodo, nodr, nodsp;

	i = n->left;
	if(i->op != ODOTINTER)
		fatal("cgen_callinter: not ODOTINTER %O", i->op);

	f = i->right;		// field
	if(f->op != ONAME)
		fatal("cgen_callinter: not ONAME %O", f->op);

	i = i->left;		// interface

	if(!i->addable) {
		tempname(&tmpi, i->type);
		cgen(i, &tmpi);
		i = &tmpi;
	}

	genlist(n->list);		// assign the args

	// Can regalloc now; i is known to be addable,
	// so the agen will be easy.
	regalloc(&nodr, types[tptr], res);
	regalloc(&nodo, types[tptr], &nodr);
	nodo.op = OINDREG;

	agen(i, &nodr);		// REG = &inter

	nodindreg(&nodsp, types[tptr], D_SP);
	nodo.xoffset += widthptr;
	cgen(&nodo, &nodsp);	// 0(SP) = 4(REG) -- i.data

	nodo.xoffset -= widthptr;
	cgen(&nodo, &nodr);	// REG = 0(REG) -- i.tab

	if(n->left->xoffset == BADWIDTH)
		fatal("cgen_callinter: badwidth");
	nodo.xoffset = n->left->xoffset + 3*widthptr + 8;
	cgen(&nodo, &nodr);	// REG = 20+offset(REG) -- i.tab->fun[f]

	// BOTCH nodr.type = fntype;
	nodr.type = n->left->type;
	ginscall(&nodr, proc);

	regfree(&nodr);
	regfree(&nodo);

	setmaxarg(n->left->type);
}

/*
 * generate function call;
 *	proc=0	normal call
 *	proc=1	goroutine run in new proc
 *	proc=2	defer call save away stack
 */
void
cgen_call(Node *n, int proc)
{
	Type *t;
	Node nod, afun;

	if(n == N)
		return;

	if(n->left->ullman >= UINF) {
		// if name involves a fn call
		// precompute the address of the fn
		tempname(&afun, types[tptr]);
		cgen(n->left, &afun);
	}

	genlist(n->list);		// assign the args
	t = n->left->type;

	setmaxarg(t);

	// call tempname pointer
	if(n->left->ullman >= UINF) {
		regalloc(&nod, types[tptr], N);
		cgen_as(&nod, &afun);
		nod.type = t;
		ginscall(&nod, proc);
		regfree(&nod);
		return;
	}

	// call pointer
	if(n->left->op != ONAME || n->left->class != PFUNC) {
		regalloc(&nod, types[tptr], N);
		cgen_as(&nod, n->left);
		nod.type = t;
		ginscall(&nod, proc);
		regfree(&nod);
		return;
	}

	// call direct
	n->left->method = 1;
	ginscall(n->left, proc);
}

/*
 * call to n has already been generated.
 * generate:
 *	res = return value from call.
 */
void
cgen_callret(Node *n, Node *res)
{
	Node nod;
	Type *fp, *t;
	Iter flist;

	t = n->left->type;
	if(t->etype == TPTR32 || t->etype == TPTR64)
		t = t->type;

	fp = structfirst(&flist, getoutarg(t));
	if(fp == T)
		fatal("cgen_callret: nil");

	memset(&nod, 0, sizeof(nod));
	nod.op = OINDREG;
	nod.val.u.reg = D_SP;
	nod.addable = 1;

	nod.xoffset = fp->width;
	nod.type = fp->type;
	cgen_as(res, &nod);
}

/*
 * call to n has already been generated.
 * generate:
 *	res = &return value from call.
 */
void
cgen_aret(Node *n, Node *res)
{
	Node nod1, nod2;
	Type *fp, *t;
	Iter flist;

	t = n->left->type;
	if(isptr[t->etype])
		t = t->type;

	fp = structfirst(&flist, getoutarg(t));
	if(fp == T)
		fatal("cgen_aret: nil");

	memset(&nod1, 0, sizeof(nod1));
	nod1.op = OINDREG;
	nod1.val.u.reg = D_SP;
	nod1.addable = 1;

	nod1.xoffset = fp->width;
	nod1.type = fp->type;

	if(res->op != OREGISTER) {
		regalloc(&nod2, types[tptr], res);
		gins(ALEAL, &nod1, &nod2);
		gins(AMOVL, &nod2, res);
		regfree(&nod2);
	} else
		gins(ALEAL, &nod1, res);
}

/*
 * generate return.
 * n->left is assignments to return values.
 */
void
cgen_ret(Node *n)
{
	genlist(n->list);		// copy out args
	if(retpc)
		gjmp(retpc);
	else
		gins(ARET, N, N);
}

/*
 * generate += *= etc.
 */
void
cgen_asop(Node *n)
{
	Node n1, n2, n3, n4;
	Node *nl, *nr;
	Prog *p1;
	Addr addr;
	int a;

	nl = n->left;
	nr = n->right;

	if(nr->ullman >= UINF && nl->ullman >= UINF) {
		tempname(&n1, nr->type);
		cgen(nr, &n1);
		n2 = *n;
		n2.right = &n1;
		cgen_asop(&n2);
		goto ret;
	}

	if(!isint[nl->type->etype])
		goto hard;
	if(!isint[nr->type->etype])
		goto hard;
	if(is64(nl->type) || is64(nr->type))
		goto hard;

	switch(n->etype) {
	case OADD:
		if(smallintconst(nr))
		if(mpgetfix(nr->val.u.xval) == 1) {
			a = optoas(OINC, nl->type);
			if(nl->addable) {
				gins(a, N, nl);
				goto ret;
			}
			if(sudoaddable(a, nl, &addr)) {
				p1 = gins(a, N, N);
				p1->to = addr;
				sudoclean();
				goto ret;
			}
		}
		break;

	case OSUB:
		if(smallintconst(nr))
		if(mpgetfix(nr->val.u.xval) == 1) {
			a = optoas(ODEC, nl->type);
			if(nl->addable) {
				gins(a, N, nl);
				goto ret;
			}
			if(sudoaddable(a, nl, &addr)) {
				p1 = gins(a, N, N);
				p1->to = addr;
				sudoclean();
				goto ret;
			}
		}
		break;
	}

	switch(n->etype) {
	case OADD:
	case OSUB:
	case OXOR:
	case OAND:
	case OOR:
		a = optoas(n->etype, nl->type);
		if(nl->addable) {
			if(smallintconst(nr)) {
				gins(a, nr, nl);
				goto ret;
			}
			regalloc(&n2, nr->type, N);
			cgen(nr, &n2);
			gins(a, &n2, nl);
			regfree(&n2);
			goto ret;
		}
		if(nr->ullman < UINF)
		if(sudoaddable(a, nl, &addr)) {
			if(smallintconst(nr)) {
				p1 = gins(a, nr, N);
				p1->to = addr;
				sudoclean();
				goto ret;
			}
			regalloc(&n2, nr->type, N);
			cgen(nr, &n2);
			p1 = gins(a, &n2, N);
			p1->to = addr;
			regfree(&n2);
			sudoclean();
			goto ret;
		}
	}

hard:
	n2.op = 0;
	n1.op = 0;
	if(nr->ullman >= nl->ullman || nl->addable) {
		mgen(nr, &n2, N);
		nr = &n2;
	} else {
		tempname(&n2, nr->type);
		cgen(nr, &n2);
		nr = &n2;
	}
	if(!nl->addable) {
		igen(nl, &n1, N);
		nl = &n1;
	}

	n3 = *n;
	n3.left = nl;
	n3.right = nr;
	n3.op = n->etype;

	mgen(&n3, &n4, N);
	gmove(&n4, nl);

	if(n1.op)
		regfree(&n1);
	mfree(&n2);
	mfree(&n4);

ret:
	;
}

int
samereg(Node *a, Node *b)
{
	if(a->op != OREGISTER)
		return 0;
	if(b->op != OREGISTER)
		return 0;
	if(a->val.u.reg != b->val.u.reg)
		return 0;
	return 1;
}

/*
 * generate division.
 * caller must set:
 *	ax = allocated AX register
 *	dx = allocated DX register
 * generates one of:
 *	res = nl / nr
 *	res = nl % nr
 * according to op.
 */
void
dodiv(int op, Node *nl, Node *nr, Node *res, Node *ax, Node *dx)
{
	int check;
	Node n1, t1, t2, t3, t4, n4, nz;
	Type *t, *t0;
	Prog *p1, *p2, *p3;

	// Have to be careful about handling
	// most negative int divided by -1 correctly.
	// The hardware will trap.
	// Also the byte divide instruction needs AH,
	// which we otherwise don't have to deal with.
	// Easiest way to avoid for int8, int16: use int32.
	// For int32 and int64, use explicit test.
	// Could use int64 hw for int32.
	t = nl->type;
	t0 = t;
	check = 0;
	if(issigned[t->etype]) {
		check = 1;
		if(isconst(nl, CTINT) && mpgetfix(nl->val.u.xval) != -1LL<<(t->width*8-1))
			check = 0;
		else if(isconst(nr, CTINT) && mpgetfix(nr->val.u.xval) != -1)
			check = 0;
	}
	if(t->width < 4) {
		if(issigned[t->etype])
			t = types[TINT32];
		else
			t = types[TUINT32];
		check = 0;
	}

	tempname(&t1, t);
	tempname(&t2, t);
	if(t0 != t) {
		tempname(&t3, t0);
		tempname(&t4, t0);
		cgen(nl, &t3);
		cgen(nr, &t4);
		// Convert.
		gmove(&t3, &t1);
		gmove(&t4, &t2);
	} else {
		cgen(nl, &t1);
		cgen(nr, &t2);
	}

	if(!samereg(ax, res) && !samereg(dx, res))
		regalloc(&n1, t, res);
	else
		regalloc(&n1, t, N);
	gmove(&t2, &n1);
	gmove(&t1, ax);
	p3 = P;
	if(check) {
		nodconst(&n4, t, -1);
		gins(optoas(OCMP, t), &n1, &n4);
		p1 = gbranch(optoas(ONE, t), T);
		nodconst(&n4, t, -1LL<<(t->width*8-1));
		gins(optoas(OCMP, t), ax, &n4);
		p2 = gbranch(optoas(ONE, t), T);
		if(op == ODIV)
			gmove(&n4, res);
		if(op == OMOD) {
			nodconst(&n4, t, 0);
			gmove(&n4, res);
		}
		p3 = gbranch(AJMP, T);
		patch(p1, pc);
		patch(p2, pc);
	}
	if(!issigned[t->etype]) {
		nodconst(&nz, t, 0);
		gmove(&nz, dx);
	} else
		gins(optoas(OEXTEND, t), N, N);
	gins(optoas(op, t), &n1, N);
	regfree(&n1);

	if(op == ODIV)
		gmove(ax, res);
	else
		gmove(dx, res);
	if(check)
		patch(p3, pc);
}

static void
savex(int dr, Node *x, Node *oldx, Node *res, Type *t)
{
	int r;

	r = reg[dr];
	nodreg(x, types[TINT32], dr);

	// save current ax and dx if they are live
	// and not the destination
	memset(oldx, 0, sizeof *oldx);
	if(r > 0 && !samereg(x, res)) {
		tempname(oldx, types[TINT32]);
		gmove(x, oldx);
	}

	regalloc(x, t, x);
}

static void
restx(Node *x, Node *oldx)
{
	regfree(x);

	if(oldx->op != 0) {
		x->type = types[TINT32];
		gmove(oldx, x);
	}
}

/*
 * generate division according to op, one of:
 *	res = nl / nr
 *	res = nl % nr
 */
void
cgen_div(int op, Node *nl, Node *nr, Node *res)
{
	Node ax, dx, oldax, olddx;
	Type *t;

	if(is64(nl->type))
		fatal("cgen_div %T", nl->type);

	if(issigned[nl->type->etype])
		t = types[TINT32];
	else
		t = types[TUINT32];
	savex(D_AX, &ax, &oldax, res, t);
	savex(D_DX, &dx, &olddx, res, t);
	dodiv(op, nl, nr, res, &ax, &dx);
	restx(&dx, &olddx);
	restx(&ax, &oldax);
}

/*
 * generate shift according to op, one of:
 *	res = nl << nr
 *	res = nl >> nr
 */
void
cgen_shift(int op, Node *nl, Node *nr, Node *res)
{
	Node n1, n2, nt, cx, oldcx, hi, lo;
	int a, w;
	Prog *p1, *p2;
	uvlong sc;

	if(nl->type->width > 4)
		fatal("cgen_shift %T", nl->type);

	w = nl->type->width * 8;

	a = optoas(op, nl->type);

	if(nr->op == OLITERAL) {
		tempname(&n2, nl->type);
		cgen(nl, &n2);
		regalloc(&n1, nl->type, res);
		gmove(&n2, &n1);
		sc = mpgetfix(nr->val.u.xval);
		if(sc >= nl->type->width*8) {
			// large shift gets 2 shifts by width
			gins(a, ncon(w-1), &n1);
			gins(a, ncon(w-1), &n1);
		} else
			gins(a, nr, &n1);
		gmove(&n1, res);
		regfree(&n1);
		return;
	}

	memset(&oldcx, 0, sizeof oldcx);
	nodreg(&cx, types[TUINT32], D_CX);
	if(reg[D_CX] > 1 && !samereg(&cx, res)) {
		tempname(&oldcx, types[TUINT32]);
		gmove(&cx, &oldcx);
	}

	if(nr->type->width > 4) {
		tempname(&nt, nr->type);
		n1 = nt;
	} else {
		nodreg(&n1, types[TUINT32], D_CX);
		regalloc(&n1, nr->type, &n1);		// to hold the shift type in CX
	}

	if(samereg(&cx, res))
		regalloc(&n2, nl->type, N);
	else
		regalloc(&n2, nl->type, res);
	if(nl->ullman >= nr->ullman) {
		cgen(nl, &n2);
		cgen(nr, &n1);
	} else {
		cgen(nr, &n1);
		cgen(nl, &n2);
	}

	// test and fix up large shifts
	if(nr->type->width > 4) {
		// delayed reg alloc
		nodreg(&n1, types[TUINT32], D_CX);
		regalloc(&n1, types[TUINT32], &n1);		// to hold the shift type in CX
		split64(&nt, &lo, &hi);
		gmove(&lo, &n1);
		gins(optoas(OCMP, types[TUINT32]), &hi, ncon(0));
		p2 = gbranch(optoas(ONE, types[TUINT32]), T);
		gins(optoas(OCMP, types[TUINT32]), &n1, ncon(w));
		p1 = gbranch(optoas(OLT, types[TUINT32]), T);
		patch(p2, pc);
	} else {
		gins(optoas(OCMP, nr->type), &n1, ncon(w));
		p1 = gbranch(optoas(OLT, types[TUINT32]), T);
	}
	if(op == ORSH && issigned[nl->type->etype]) {
		gins(a, ncon(w-1), &n2);
	} else {
		gmove(ncon(0), &n2);
	}
	patch(p1, pc);
	gins(a, &n1, &n2);

	if(oldcx.op != 0)
		gmove(&oldcx, &cx);

	gmove(&n2, res);

	regfree(&n1);
	regfree(&n2);
}

/*
 * generate byte multiply:
 *	res = nl * nr
 * no byte multiply instruction so have to do
 * 16-bit multiply and take bottom half.
 */
void
cgen_bmul(int op, Node *nl, Node *nr, Node *res)
{
	Node n1b, n2b, n1w, n2w;
	Type *t;
	int a;

	if(nl->ullman >= nr->ullman) {
		regalloc(&n1b, nl->type, res);
		cgen(nl, &n1b);
		regalloc(&n2b, nr->type, N);
		cgen(nr, &n2b);
	} else {
		regalloc(&n2b, nr->type, N);
		cgen(nr, &n2b);
		regalloc(&n1b, nl->type, res);
		cgen(nl, &n1b);
	}

	// copy from byte to short registers
	t = types[TUINT16];
	if(issigned[nl->type->etype])
		t = types[TINT16];

	regalloc(&n2w, t, &n2b);
	cgen(&n2b, &n2w);

	regalloc(&n1w, t, &n1b);
	cgen(&n1b, &n1w);

	a = optoas(op, t);
	gins(a, &n2w, &n1w);
	cgen(&n1w, &n1b);
	cgen(&n1b, res);

	regfree(&n1w);
	regfree(&n2w);
	regfree(&n1b);
	regfree(&n2b);
}

static int
regcmp(const void *va, const void *vb)
{
	Node *ra, *rb;

	ra = (Node*)va;
	rb = (Node*)vb;
	return ra->local - rb->local;
}

static	Prog*	throwpc;

// We're only going to bother inlining if we can
// convert all the arguments to 32 bits safely.  Can we?
static int
fix64(NodeList *nn, int n)
{
	NodeList *l;
	Node *r;
	int i;
	
	l = nn;
	for(i=0; i<n; i++) {
		r = l->n->right;
		if(is64(r->type) && !smallintconst(r)) {
			if(r->op == OCONV)
				r = r->left;
			if(is64(r->type))
				return 0;
		}
		l = l->next;
	}
	return 1;
}

void
getargs(NodeList *nn, Node *reg, int n)
{
	NodeList *l;
	Node *r;
	int i;

	throwpc = nil;

	l = nn;
	for(i=0; i<n; i++) {
		r = l->n->right;
		if(is64(r->type)) {
			if(r->op == OCONV)
				r = r->left;
			else if(smallintconst(r))
				r->type = types[TUINT32];
			if(is64(r->type))
				fatal("getargs");
		}
		if(!smallintconst(r) && !isslice(r->type)) {
			if(i < 3)	// AX CX DX
				nodreg(reg+i, r->type, D_AX+i);
			else
				reg[i].op = OXXX;
			regalloc(reg+i, r->type, reg+i);
			cgen(r, reg+i);
		} else
			reg[i] = *r;
		if(reg[i].local != 0)
			yyerror("local used");
		reg[i].local = l->n->left->xoffset;
		l = l->next;
	}
	qsort((void*)reg, n, sizeof(*reg), regcmp);
	for(i=0; i<n; i++)
		reg[i].local = 0;
}

void
cmpandthrow(Node *nl, Node *nr)
{
	vlong cl;
	Prog *p1;
	int op;
	Node *c, n1;
	Type *t;

	op = OLE;
	if(smallintconst(nl)) {
		cl = mpgetfix(nl->val.u.xval);
		if(cl == 0)
			return;
		if(smallintconst(nr))
			return;
		// put the constant on the right
		op = brrev(op);
		c = nl;
		nl = nr;
		nr = c;
	}
	
	// Arguments are known not to be 64-bit,
	// but they might be smaller than 32 bits.
	// Check if we need to use a temporary.
	// At least one of the arguments is 32 bits
	// (the len or cap) so one temporary suffices.
	n1.op = OXXX;
	t = types[TUINT32];
	if(nl->type->width != t->width) {
		regalloc(&n1, t, nl);
		gmove(nl, &n1);
		nl = &n1;
	} else if(nr->type->width != t->width) {
		regalloc(&n1, t, nr);
		gmove(nr, &n1);
		nr = &n1;
	}
	gins(optoas(OCMP, t), nl, nr);
	if(n1.op != OXXX)
		regfree(&n1);
	if(throwpc == nil) {
		p1 = gbranch(optoas(op, t), T);
		throwpc = pc;
		ginscall(panicslice, 0);
		patch(p1, pc);
	} else {
		op = brcom(op);
		p1 = gbranch(optoas(op, t), T);
		patch(p1, throwpc);
	}
}

int
sleasy(Node *n)
{
	if(n->op != ONAME)
		return 0;
	if(!n->addable)
		return 0;
	return 1;
}

// generate inline code for
//	slicearray
//	sliceslice
//	arraytoslice
int
cgen_inline(Node *n, Node *res)
{
	Node nodes[5];
	Node n1, n2, nres, ntemp;
	vlong v;
	int i, narg, nochk;

	if(n->op != OCALLFUNC)
		goto no;
	if(!n->left->addable)
		goto no;
	if(n->left->sym == S)
		goto no;
	if(n->left->sym->pkg != runtimepkg)
		goto no;
	if(strcmp(n->left->sym->name, "slicearray") == 0)
		goto slicearray;
	if(strcmp(n->left->sym->name, "sliceslice") == 0) {
		narg = 4;
		goto sliceslice;
	}
	if(strcmp(n->left->sym->name, "sliceslice1") == 0) {
		narg = 3;
		goto sliceslice;
	}
	goto no;

slicearray:
	if(!sleasy(res))
		goto no;
	if(!fix64(n->list, 5))
		goto no;
	getargs(n->list, nodes, 5);

	// if(hb[3] > nel[1]) goto throw
	cmpandthrow(&nodes[3], &nodes[1]);

	// if(lb[2] > hb[3]) goto throw
	cmpandthrow(&nodes[2], &nodes[3]);

	// len = hb[3] - lb[2] (destroys hb)
	n2 = *res;
	n2.xoffset += Array_nel;
	n2.type = types[TUINT32];

	if(smallintconst(&nodes[3]) && smallintconst(&nodes[2])) {
		v = mpgetfix(nodes[3].val.u.xval) -
			mpgetfix(nodes[2].val.u.xval);
		nodconst(&n1, types[TUINT32], v);
		gins(optoas(OAS, types[TUINT32]), &n1, &n2);
	} else {
		regalloc(&n1, types[TUINT32], &nodes[3]);
		gmove(&nodes[3], &n1);
		if(!smallintconst(&nodes[2]) || mpgetfix(nodes[2].val.u.xval) != 0)
			gins(optoas(OSUB, types[TUINT32]), &nodes[2], &n1);
		gins(optoas(OAS, types[TUINT32]), &n1, &n2);
		regfree(&n1);
	}

	// cap = nel[1] - lb[2] (destroys nel)
	n2 = *res;
	n2.xoffset += Array_cap;
	n2.type = types[TUINT32];

	if(smallintconst(&nodes[1]) && smallintconst(&nodes[2])) {
		v = mpgetfix(nodes[1].val.u.xval) -
			mpgetfix(nodes[2].val.u.xval);
		nodconst(&n1, types[TUINT32], v);
		gins(optoas(OAS, types[TUINT32]), &n1, &n2);
	} else {
		regalloc(&n1, types[TUINT32], &nodes[1]);
		gmove(&nodes[1], &n1);
		if(!smallintconst(&nodes[2]) || mpgetfix(nodes[2].val.u.xval) != 0)
			gins(optoas(OSUB, types[TUINT32]), &nodes[2], &n1);
		gins(optoas(OAS, types[TUINT32]), &n1, &n2);
		regfree(&n1);
	}

	// if slice could be too big, dereference to
	// catch nil array pointer.
	if(nodes[0].op == OREGISTER && nodes[0].type->type->width >= unmappedzero) {
		n2 = nodes[0];
		n2.xoffset = 0;
		n2.op = OINDREG;
		n2.type = types[TUINT8];
		gins(ATESTB, nodintconst(0), &n2);
	}

	// ary = old[0] + (lb[2] * width[4]) (destroys old)
	n2 = *res;
	n2.xoffset += Array_array;
	n2.type = types[tptr];

	if(smallintconst(&nodes[2]) && smallintconst(&nodes[4])) {
		v = mpgetfix(nodes[2].val.u.xval) *
			mpgetfix(nodes[4].val.u.xval);
		if(v != 0) {
			nodconst(&n1, types[tptr], v);
			gins(optoas(OADD, types[tptr]), &n1, &nodes[0]);
		}
	} else {
		regalloc(&n1, types[tptr], &nodes[2]);
		gmove(&nodes[2], &n1);
		if(!smallintconst(&nodes[4]) || mpgetfix(nodes[4].val.u.xval) != 1)
			gins(optoas(OMUL, types[tptr]), &nodes[4], &n1);
		gins(optoas(OADD, types[tptr]), &n1, &nodes[0]);
		regfree(&n1);
	}
	gins(optoas(OAS, types[tptr]), &nodes[0], &n2);

	for(i=0; i<5; i++) {
		if(nodes[i].op == OREGISTER)
			regfree(&nodes[i]);
	}
	return 1;

sliceslice:
	if(!fix64(n->list, narg))
		goto no;
	nochk = n->etype;  // skip bounds checking
	ntemp.op = OXXX;
	if(!sleasy(n->list->n->right)) {
		Node *n0;
		
		n0 = n->list->n->right;
		tempname(&ntemp, res->type);
		cgen(n0, &ntemp);
		n->list->n->right = &ntemp;
		getargs(n->list, nodes, narg);
		n->list->n->right = n0;
	} else
		getargs(n->list, nodes, narg);

	nres = *res;		// result
	if(!sleasy(res)) {
		if(ntemp.op == OXXX)
			tempname(&ntemp, res->type);
		nres = ntemp;
	}

	if(narg == 3) {	// old[lb:]
		// move width to where it would be for old[lb:hb]
		nodes[3] = nodes[2];
		nodes[2].op = OXXX;
		
		// if(lb[1] > old.nel[0]) goto throw;
		n2 = nodes[0];
		n2.xoffset += Array_nel;
		n2.type = types[TUINT32];
		if(!nochk)
			cmpandthrow(&nodes[1], &n2);

		// ret.nel = old.nel[0]-lb[1];
		n2 = nodes[0];
		n2.xoffset += Array_nel;
		n2.type = types[TUINT32];
	
		regalloc(&n1, types[TUINT32], N);
		gins(optoas(OAS, types[TUINT32]), &n2, &n1);
		if(!smallintconst(&nodes[1]) || mpgetfix(nodes[1].val.u.xval) != 0)
			gins(optoas(OSUB, types[TUINT32]), &nodes[1], &n1);
	
		n2 = nres;
		n2.xoffset += Array_nel;
		n2.type = types[TUINT32];
		gins(optoas(OAS, types[TUINT32]), &n1, &n2);
		regfree(&n1);
	} else {	// old[lb:hb]
		n2 = nodes[0];
		n2.xoffset += Array_cap;
		n2.type = types[TUINT32];
		if (!nochk) {
			// if(hb[2] > old.cap[0]) goto throw;
			cmpandthrow(&nodes[2], &n2);
			// if(lb[1] > hb[2]) goto throw;
			cmpandthrow(&nodes[1], &nodes[2]);
		}

		// ret.len = hb[2]-lb[1]; (destroys hb[2])
		n2 = nres;
		n2.xoffset += Array_nel;
		n2.type = types[TUINT32];

		if(smallintconst(&nodes[2]) && smallintconst(&nodes[1])) {
			v = mpgetfix(nodes[2].val.u.xval) -
				mpgetfix(nodes[1].val.u.xval);
			nodconst(&n1, types[TUINT32], v);
			gins(optoas(OAS, types[TUINT32]), &n1, &n2);
		} else {
			regalloc(&n1, types[TUINT32], &nodes[2]);
			gmove(&nodes[2], &n1);
			if(!smallintconst(&nodes[1]) || mpgetfix(nodes[1].val.u.xval) != 0)
				gins(optoas(OSUB, types[TUINT32]), &nodes[1], &n1);
			gins(optoas(OAS, types[TUINT32]), &n1, &n2);
			regfree(&n1);
		}
	}

	// ret.cap = old.cap[0]-lb[1]; (uses hb[2])
	n2 = nodes[0];
	n2.xoffset += Array_cap;
	n2.type = types[TUINT32];

	regalloc(&n1, types[TUINT32], &nodes[2]);
	gins(optoas(OAS, types[TUINT32]), &n2, &n1);
	if(!smallintconst(&nodes[1]) || mpgetfix(nodes[1].val.u.xval) != 0)
		gins(optoas(OSUB, types[TUINT32]), &nodes[1], &n1);

	n2 = nres;
	n2.xoffset += Array_cap;
	n2.type = types[TUINT32];
	gins(optoas(OAS, types[TUINT32]), &n1, &n2);
	regfree(&n1);

	// ret.array = old.array[0]+lb[1]*width[3]; (uses lb[1])
	n2 = nodes[0];
	n2.xoffset += Array_array;
	n2.type = types[tptr];

	regalloc(&n1, types[tptr], &nodes[1]);
	if(smallintconst(&nodes[1]) && smallintconst(&nodes[3])) {
		gins(optoas(OAS, types[tptr]), &n2, &n1);
		v = mpgetfix(nodes[1].val.u.xval) *
			mpgetfix(nodes[3].val.u.xval);
		if(v != 0) {
			nodconst(&n2, types[tptr], v);
			gins(optoas(OADD, types[tptr]), &n2, &n1);
		}
	} else {
		gmove(&nodes[1], &n1);
		if(!smallintconst(&nodes[3]) || mpgetfix(nodes[3].val.u.xval) != 1)
			gins(optoas(OMUL, types[tptr]), &nodes[3], &n1);
		gins(optoas(OADD, types[tptr]), &n2, &n1);
	}

	n2 = nres;
	n2.xoffset += Array_array;
	n2.type = types[tptr];
	gins(optoas(OAS, types[tptr]), &n1, &n2);
	regfree(&n1);

	for(i=0; i<4; i++) {
		if(nodes[i].op == OREGISTER)
			regfree(&nodes[i]);
	}

	if(!sleasy(res)) {
		cgen(&nres, res);
	}
	return 1;

no:
	return 0;
}
