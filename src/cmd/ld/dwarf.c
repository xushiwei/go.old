// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO/NICETOHAVE:
//   - eliminate DW_CLS_ if not used
//   - package info in compilation units
//   - assign global variables and types to their packages
//   - gdb uses c syntax, meaning clumsy quoting is needed for go identifiers. eg
//     ptype struct '[]uint8' and qualifiers need to be quoted away
//   - lexical scoping is lost, so gdb gets confused as to which 'main.i' you mean.
//   - file:line info for variables
//   - make strings a typedef so prettyprinters can see the underlying string type
//
#include	"l.h"
#include	"lib.h"
#include	"../ld/dwarf.h"
#include	"../ld/dwarf_defs.h"
#include	"../ld/elf.h"
#include	"../ld/macho.h"
#include	"../ld/pe.h"

/*
 * Offsets and sizes of the debug_* sections in the cout file.
 */

static vlong abbrevo;
static vlong abbrevsize;
static vlong lineo;
static vlong linesize;
static vlong infoo;	// also the base for DWDie->offs and reference attributes.
static vlong infosize;
static vlong frameo;
static vlong framesize;
static vlong pubnameso;
static vlong pubnamessize;
static vlong pubtypeso;
static vlong pubtypessize;
static vlong arangeso;
static vlong arangessize;
static vlong gdbscripto;
static vlong gdbscriptsize;

static char  gdbscript[1024];

/*
 *  Basic I/O
 */

static void
addrput(vlong addr)
{
	switch(PtrSize) {
	case 4:
		LPUT(addr);
		break;
	case 8:
		VPUT(addr);
		break;
	}
}

static int
uleb128enc(uvlong v, char* dst)
{
	uint8 c, len;

	len = 0;
	do {
		c = v & 0x7f;
		v >>= 7;
		if (v)
			c |= 0x80;
		if (dst)
			*dst++ = c;
		len++;
	} while (c & 0x80);
	return len;
};

static int
sleb128enc(vlong v, char *dst)
{
	uint8 c, s, len;

	len = 0;
	do {
		c = v & 0x7f;
		s = v & 0x40;
		v >>= 7;
		if ((v != -1 || !s) && (v != 0 || s))
			c |= 0x80;
		if (dst)
			*dst++ = c;
		len++;
	} while(c & 0x80);
	return len;
}

static void
uleb128put(vlong v)
{
	char buf[10];
	strnput(buf, uleb128enc(v, buf));
}

static void
sleb128put(vlong v)
{
	char buf[10];
	strnput(buf, sleb128enc(v, buf));
}

/*
 * Defining Abbrevs.  This is hardcoded, and there will be
 * only a handful of them.  The DWARF spec places no restriction on
 * the ordering of atributes in the Abbrevs and DIEs, and we will
 * always write them out in the order of declaration in the abbrev.
 * This implementation relies on tag, attr < 127, so they serialize as
 * a char.  Higher numbered user-defined tags or attributes can be used
 * for storing internal data but won't be serialized.
 */
typedef struct DWAttrForm DWAttrForm;
struct DWAttrForm {
	uint8 attr;
	uint8 form;
};

// Index into the abbrevs table below.
// Keep in sync with ispubname() and ispubtype() below.
// ispubtype considers >= NULLTYPE public
enum
{
	DW_ABRV_NULL,
	DW_ABRV_COMPUNIT,
	DW_ABRV_FUNCTION,
	DW_ABRV_VARIABLE,
	DW_ABRV_AUTO,
	DW_ABRV_PARAM,
	DW_ABRV_STRUCTFIELD,
	DW_ABRV_FUNCTYPEPARAM,
	DW_ABRV_DOTDOTDOT,
	DW_ABRV_ARRAYRANGE,
	DW_ABRV_NULLTYPE,
	DW_ABRV_BASETYPE,
	DW_ABRV_ARRAYTYPE,
	DW_ABRV_CHANTYPE,
	DW_ABRV_FUNCTYPE,
	DW_ABRV_IFACETYPE,
	DW_ABRV_MAPTYPE,
	DW_ABRV_PTRTYPE,
	DW_ABRV_SLICETYPE,
	DW_ABRV_STRINGTYPE,
	DW_ABRV_STRUCTTYPE,
	DW_ABRV_TYPEDECL,
	DW_NABRV
};

typedef struct DWAbbrev DWAbbrev;
static struct DWAbbrev {
	uint8 tag;
	uint8 children;
	DWAttrForm attr[30];
} abbrevs[DW_NABRV] = {
	/* The mandatory DW_ABRV_NULL entry. */
	{ 0 },
	/* COMPUNIT */
	{
		DW_TAG_compile_unit, DW_CHILDREN_yes,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_language,	 DW_FORM_data1,
		DW_AT_low_pc,	 DW_FORM_addr,
		DW_AT_high_pc,	 DW_FORM_addr,
		DW_AT_stmt_list, DW_FORM_data4,
		0, 0
	},
	/* FUNCTION */
	{
		DW_TAG_subprogram, DW_CHILDREN_yes,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_low_pc,	 DW_FORM_addr,
		DW_AT_high_pc,	 DW_FORM_addr,
		DW_AT_external,	 DW_FORM_flag,
		0, 0
	},
	/* VARIABLE */
	{
		DW_TAG_variable, DW_CHILDREN_no,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_location,	 DW_FORM_block1,
		DW_AT_type,	 DW_FORM_ref_addr,
		DW_AT_external,	 DW_FORM_flag,
		0, 0
	},
	/* AUTO */
	{
		DW_TAG_variable, DW_CHILDREN_no,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_location,	 DW_FORM_block1,
		DW_AT_type,	 DW_FORM_ref_addr,
		0, 0
	},
	/* PARAM */
	{
		DW_TAG_formal_parameter, DW_CHILDREN_no,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_location,	 DW_FORM_block1,
		DW_AT_type,	 DW_FORM_ref_addr,
		0, 0
	},
	/* STRUCTFIELD */
	{
		DW_TAG_member,	DW_CHILDREN_no,
		DW_AT_name,	DW_FORM_string,
		DW_AT_data_member_location, DW_FORM_block1,
		DW_AT_type,	 DW_FORM_ref_addr,
		0, 0
	},
	/* FUNCTYPEPARAM */
	{
		DW_TAG_formal_parameter, DW_CHILDREN_no,
		// No name!
		DW_AT_type,	 DW_FORM_ref_addr,
		0, 0
	},

	/* DOTDOTDOT */
	{
		DW_TAG_unspecified_parameters, DW_CHILDREN_no,
		0, 0
	},
	/* ARRAYRANGE */
	{
		DW_TAG_subrange_type, DW_CHILDREN_no,
		// No name!
		DW_AT_type,	 DW_FORM_ref_addr,
		DW_AT_upper_bound, DW_FORM_data1,
		0, 0
	},

	// Below here are the types considered public by ispubtype
	/* NULLTYPE */
	{
		DW_TAG_unspecified_type, DW_CHILDREN_no,
		DW_AT_name,	DW_FORM_string,
		0, 0
	},
	/* BASETYPE */
	{
		DW_TAG_base_type, DW_CHILDREN_no,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_encoding,	 DW_FORM_data1,
		DW_AT_byte_size, DW_FORM_data1,
		0, 0
	},
	/* ARRAYTYPE */
	// child is subrange with upper bound
	{
		DW_TAG_array_type, DW_CHILDREN_yes,
		DW_AT_name,	DW_FORM_string,
		DW_AT_type,	DW_FORM_ref_addr,
		DW_AT_byte_size, DW_FORM_udata,
		0, 0
	},

	/* CHANTYPE */
	{
		DW_TAG_typedef, DW_CHILDREN_no,
		DW_AT_name,	DW_FORM_string,
		DW_AT_type,	DW_FORM_ref_addr,
		0, 0
	},

	/* FUNCTYPE */
	{
		DW_TAG_subroutine_type, DW_CHILDREN_yes,
		DW_AT_name,	DW_FORM_string,
//		DW_AT_type,	DW_FORM_ref_addr,
		0, 0
	},

	/* IFACETYPE */
	{
		DW_TAG_typedef, DW_CHILDREN_yes,
		DW_AT_name,	 DW_FORM_string,
		DW_AT_type,	DW_FORM_ref_addr,
		0, 0
	},

	/* MAPTYPE */
	{
		DW_TAG_typedef, DW_CHILDREN_no,
		DW_AT_name,	DW_FORM_string,
		DW_AT_type,	DW_FORM_ref_addr,
		0, 0
	},

	/* PTRTYPE */
	{
		DW_TAG_pointer_type, DW_CHILDREN_no,
		DW_AT_name,	DW_FORM_string,
		DW_AT_type,	DW_FORM_ref_addr,
		0, 0
	},

	/* SLICETYPE */
	{
		DW_TAG_structure_type, DW_CHILDREN_yes,
		DW_AT_name,	DW_FORM_string,
		DW_AT_byte_size, DW_FORM_udata,
		0, 0
	},

	/* STRINGTYPE */
	{
		DW_TAG_structure_type, DW_CHILDREN_yes,
		DW_AT_name,	DW_FORM_string,
		DW_AT_byte_size, DW_FORM_udata,
		0, 0
	},

	/* STRUCTTYPE */
	{
		DW_TAG_structure_type, DW_CHILDREN_yes,
		DW_AT_name,	DW_FORM_string,
		DW_AT_byte_size, DW_FORM_udata,
		0, 0
	},

	/* TYPEDECL */
	{
		DW_TAG_typedef, DW_CHILDREN_no,
		DW_AT_name,	DW_FORM_string,
		DW_AT_type,	DW_FORM_ref_addr,
		0, 0
	},
};

static void
writeabbrev(void)
{
	int i, n;

	abbrevo = cpos();
	for (i = 1; i < DW_NABRV; i++) {
		// See section 7.5.3
		uleb128put(i);
		uleb128put(abbrevs[i].tag);
		cput(abbrevs[i].children);
		// 0 is not a valid attr or form, and DWAbbrev.attr is
		// 0-terminated, so we can treat it as a string
		n = strlen((char*)abbrevs[i].attr) / 2;
		strnput((char*)abbrevs[i].attr,
			(n+1) * sizeof(DWAttrForm));
	}
	cput(0);
	abbrevsize = cpos() - abbrevo;
}

/*
 * Debugging Information Entries and their attributes.
 */

enum
{
	HASHSIZE = 107
};

static uint32
hashstr(char* s)
{
	uint32 h;

	h = 0;
	while (*s)
		h = h+h+h + *s++;
	return h % HASHSIZE;
}

// For DW_CLS_string and _block, value should contain the length, and
// data the data, for _reference, value is 0 and data is a DWDie* to
// the referenced instance, for all others, value is the whole thing
// and data is null.

typedef struct DWAttr DWAttr;
struct DWAttr {
	DWAttr *link;
	uint8 atr;  // DW_AT_
	uint8 cls;  // DW_CLS_
	vlong value;
	char *data;
};

typedef struct DWDie DWDie;
struct DWDie {
	int abbrev;
	DWDie *link;
	DWDie *child;
	DWAttr *attr;
	// offset into .debug_info section, i.e relative to
	// infoo. only valid after call to putdie()
	vlong offs;
	DWDie **hash;  // optional index of children by name, enabled by mkindex()
	DWDie *hlink;  // bucket chain in parent's index
};

/*
 * Root DIEs for compilation units, types and global variables.
 */

static DWDie dwroot;
static DWDie dwtypes;
static DWDie dwglobals;

static DWAttr*
newattr(DWDie *die, uint8 attr, int cls, vlong value, char *data)
{
	DWAttr *a;

	a = mal(sizeof *a);
	a->link = die->attr;
	die->attr = a;
	a->atr = attr;
	a->cls = cls;
	a->value = value;
	a->data = data;
	return a;
}

// Each DIE (except the root ones) has at least 1 attribute: its
// name. getattr moves the desired one to the front so
// frequently searched ones are found faster.
static DWAttr*
getattr(DWDie *die, uint8 attr)
{
	DWAttr *a, *b;

	if (die->attr->atr == attr)
		return die->attr;

	a = die->attr;
	b = a->link;
	while (b != nil) {
		if (b->atr == attr) {
			a->link = b->link;
			b->link = die->attr;
			die->attr = b;
			return b;
		}
		a = b;
		b = b->link;
	}
	return nil;
}

// Every DIE has at least a DW_AT_name attribute (but it will only be
// written out if it is listed in the abbrev).	If its parent is
// keeping an index, the new DIE will be inserted there.
static DWDie*
newdie(DWDie *parent, int abbrev, char *name)
{
	DWDie *die;
	int h;

	die = mal(sizeof *die);
	die->abbrev = abbrev;
	die->link = parent->child;
	parent->child = die;

	newattr(die, DW_AT_name, DW_CLS_STRING, strlen(name), name);

	if (parent->hash) {
		h = hashstr(name);
		die->hlink = parent->hash[h];
		parent->hash[h] = die;
	}

	return die;
}

static void
mkindex(DWDie *die)
{
	die->hash = mal(HASHSIZE * sizeof(DWDie*));
}

// Find child by AT_name using hashtable if available or linear scan
// if not.
static DWDie*
find(DWDie *die, char* name)
{
	DWDie *a, *b;
	int h;

	if (die->hash == nil) {
		for (a = die->child; a != nil; a = a->link)
			if (strcmp(name, getattr(a, DW_AT_name)->data) == 0)
				return a;
		return nil;
	}

	h = hashstr(name);
	a = die->hash[h];

	if (a == nil)
		return nil;


	if (strcmp(name, getattr(a, DW_AT_name)->data) == 0)
		return a;

	// Move found ones to head of the list.
	b = a->hlink;
	while (b != nil) {
		if (strcmp(name, getattr(b, DW_AT_name)->data) == 0) {
			a->hlink = b->hlink;
			b->hlink = die->hash[h];
			die->hash[h] = b;
			return b;
		}
		a = b;
		b = b->hlink;
	}
	return nil;
}

static DWDie*
find_or_diag(DWDie *die, char* name)
{
	DWDie *r;
	r = find(die, name);
	if (r == nil) {
		diag("dwarf find: %s has no %s", getattr(die, DW_AT_name)->data, name);
		errorexit();
	}
	return r;
}

static DWAttr*
newrefattr(DWDie *die, uint8 attr, DWDie* ref)
{
	if (ref == nil)
		return nil;
	return newattr(die, attr, DW_CLS_REFERENCE, 0, (char*)ref);
}

static int fwdcount;

static void
putattr(int form, int cls, vlong value, char *data)
{
	switch(form) {
	case DW_FORM_addr:	// address
		addrput(value);
		break;

	case DW_FORM_block1:	// block
		value &= 0xff;
		cput(value);
		while(value--)
			cput(*data++);
		break;

	case DW_FORM_block2:	// block
		value &= 0xffff;
		WPUT(value);
		while(value--)
			cput(*data++);
		break;

	case DW_FORM_block4:	// block
		value &= 0xffffffff;
		LPUT(value);
		while(value--)
			cput(*data++);
		break;

	case DW_FORM_block:	// block
		uleb128put(value);
		while(value--)
			cput(*data++);
		break;

	case DW_FORM_data1:	// constant
		cput(value);
		break;

	case DW_FORM_data2:	// constant
		WPUT(value);
		break;

	case DW_FORM_data4:	// constant, {line,loclist,mac,rangelist}ptr
		LPUT(value);
		break;

	case DW_FORM_data8:	// constant, {line,loclist,mac,rangelist}ptr
		VPUT(value);
		break;

	case DW_FORM_sdata:	// constant
		sleb128put(value);
		break;

	case DW_FORM_udata:	// constant
		uleb128put(value);
		break;

	case DW_FORM_string:	// string
		strnput(data, value+1);
		break;

	case DW_FORM_flag:	// flag
		cput(value?1:0);
		break;

	case DW_FORM_ref_addr:	// reference to a DIE in the .info section
		if (data == nil) {
			diag("dwarf: null reference");
			LPUT(0);  // invalid dwarf, gdb will complain.
		} else {
			if (((DWDie*)data)->offs == 0)
				fwdcount++;
			LPUT(((DWDie*)data)->offs);
		}
		break;

	case DW_FORM_ref1:	// reference within the compilation unit
	case DW_FORM_ref2:	// reference
	case DW_FORM_ref4:	// reference
	case DW_FORM_ref8:	// reference
	case DW_FORM_ref_udata:	// reference

	case DW_FORM_strp:	// string
	case DW_FORM_indirect:	// (see Section 7.5.3)
	default:
		diag("dwarf: unsupported attribute form %d / class %d", form, cls);
		errorexit();
	}
}

// Note that we can (and do) add arbitrary attributes to a DIE, but
// only the ones actually listed in the Abbrev will be written out.
static void
putattrs(int abbrev, DWAttr* attr)
{
	DWAttr *attrs[DW_AT_recursive + 1];
	DWAttrForm* af;

	memset(attrs, 0, sizeof attrs);
	for( ; attr; attr = attr->link)
		if (attr->atr < nelem(attrs))
			attrs[attr->atr] = attr;

	for(af = abbrevs[abbrev].attr; af->attr; af++)
		if (attrs[af->attr])
			putattr(af->form,
				attrs[af->attr]->cls,
				attrs[af->attr]->value,
				attrs[af->attr]->data);
		else
			putattr(af->form, 0, 0, 0);
}

static void putdie(DWDie* die);

static void
putdies(DWDie* die)
{
	for(; die; die = die->link)
		putdie(die);
}

static void
putdie(DWDie* die)
{
	die->offs = cpos() - infoo;
	uleb128put(die->abbrev);
	putattrs(die->abbrev, die->attr);
	if (abbrevs[die->abbrev].children) {
		putdies(die->child);
		cput(0);
	}
}

static void
reverselist(DWDie** list)
{
	DWDie *curr, *prev;

	curr = *list;
	prev = nil;
	while(curr != nil) {
		DWDie* next = curr->link;
		curr->link = prev;
		prev = curr;
		curr = next;
	}
	*list = prev;
}

static void
reversetree(DWDie** list)
{
	 DWDie *die;

	 reverselist(list);
	 for (die = *list; die != nil; die = die->link)
		 if (abbrevs[die->abbrev].children)
			 reversetree(&die->child);
}

static void
newmemberoffsetattr(DWDie *die, int32 offs)
{
	char block[10];
	int i;

	i = 0;
	if (offs != 0) {
		block[i++] = DW_OP_consts;
		i += sleb128enc(offs, block+i);
		block[i++] = DW_OP_plus;
	}
	newattr(die, DW_AT_data_member_location, DW_CLS_BLOCK, i, mal(i));
	memmove(die->attr->data, block, i);
}

// GDB doesn't like DW_FORM_addr for DW_AT_location, so emit a
// location expression that evals to a const.
static void
newabslocexprattr(DWDie *die, vlong addr)
{
	char block[10];
	int i;

	i = 0;
	block[i++] = DW_OP_constu;
	i += uleb128enc(addr, block+i);
	newattr(die, DW_AT_location, DW_CLS_BLOCK, i, mal(i));
	memmove(die->attr->data, block, i);
}

// Decoding the type.* symbols.	 This has to be in sync with
// ../../pkg/runtime/type.go, or more specificaly, with what
// ../gc/reflect.c stuffs in these.

enum {
	KindBool = 1,
	KindInt,
	KindInt8,
	KindInt16,
	KindInt32,
	KindInt64,
	KindUint,
	KindUint8,
	KindUint16,
	KindUint32,
	KindUint64,
	KindUintptr,
	KindFloat32,
	KindFloat64,
	KindComplex64,
	KindComplex128,
	KindArray,
	KindChan,
	KindFunc,
	KindInterface,
	KindMap,
	KindPtr,
	KindSlice,
	KindString,
	KindStruct,
	KindUnsafePointer,

	KindNoPointers = 1<<7,

	// size of Type interface header + CommonType structure.
	CommonSize = 2*PtrSize+ 5*PtrSize + 8,
};

static Reloc*
decode_reloc(Sym *s, int32 off)
{
	int i;

	for (i = 0; i < s->nr; i++)
		if (s->r[i].off == off)
			return s->r + i;
	return nil;
}

static Sym*
decode_reloc_sym(Sym *s, int32 off)
{
	Reloc *r;

	r = decode_reloc(s,off);
	if (r == nil)
		return nil;
	return r->sym;
}

static uvlong
decode_inuxi(uchar* p, int sz)
{
	uint64 v;
	uint32 l;
	uchar *cast, *inuxi;
	int i;

	v = l = 0;
	cast = nil;
	inuxi = nil;
	switch (sz) {
	case 2:
		cast = (uchar*)&l;
		inuxi = inuxi2;
		break;
	case 4:
		cast = (uchar*)&l;
		inuxi = inuxi4;
		break;
	case 8:
		cast = (uchar*)&v;
		inuxi = inuxi8;
		break;
	default:
		diag("dwarf: decode inuxi %d", sz);
		errorexit();
	}
	for (i = 0; i < sz; i++)
		cast[inuxi[i]] = p[i];
	if (sz == 8)
		return v;
	return l;
}

// Type.commonType.kind
static uint8
decodetype_kind(Sym *s)
{
	return s->p[3*PtrSize + 7] & ~KindNoPointers;	//  0x13 / 0x1f
}

// Type.commonType.size
static vlong
decodetype_size(Sym *s)
{
	return decode_inuxi(s->p + 2*PtrSize, PtrSize);	 // 0x8 / 0x10
}

// Type.ArrayType.elem and Type.SliceType.Elem
static Sym*
decodetype_arrayelem(Sym *s)
{
	return decode_reloc_sym(s, CommonSize);	// 0x1c / 0x30
}

static vlong
decodetype_arraylen(Sym *s)
{
	return decode_inuxi(s->p + CommonSize+PtrSize, PtrSize);
}

// Type.PtrType.elem
static Sym*
decodetype_ptrelem(Sym *s)
{
	return decode_reloc_sym(s, CommonSize);	// 0x1c / 0x30
}

// Type.MapType.key, elem
static Sym*
decodetype_mapkey(Sym *s)
{
	return decode_reloc_sym(s, CommonSize);	// 0x1c / 0x30
}
static Sym*
decodetype_mapvalue(Sym *s)
{
	return decode_reloc_sym(s, CommonSize+PtrSize);	// 0x20 / 0x38
}

// Type.ChanType.elem
static Sym*
decodetype_chanelem(Sym *s)
{
	return decode_reloc_sym(s, CommonSize);	// 0x1c / 0x30
}

// Type.FuncType.dotdotdot
static int
decodetype_funcdotdotdot(Sym *s)
{
	return s->p[CommonSize];
}

// Type.FuncType.in.len
static int
decodetype_funcincount(Sym *s)
{
	return decode_inuxi(s->p + CommonSize+2*PtrSize, 4);
}

static int
decodetype_funcoutcount(Sym *s)
{
	return decode_inuxi(s->p + CommonSize+3*PtrSize + 2*4, 4);
}

static Sym*
decodetype_funcintype(Sym *s, int i)
{
	Reloc *r;

	r = decode_reloc(s, CommonSize + PtrSize);
	if (r == nil)
		return nil;
	return decode_reloc_sym(r->sym, r->add + i * PtrSize);
}

static Sym*
decodetype_funcouttype(Sym *s, int i)
{
	Reloc *r;

	r = decode_reloc(s, CommonSize + 2*PtrSize + 2*4);
	if (r == nil)
		return nil;
	return decode_reloc_sym(r->sym, r->add + i * PtrSize);
}

// Type.StructType.fields.Slice::len
static int
decodetype_structfieldcount(Sym *s)
{
	return decode_inuxi(s->p + CommonSize + PtrSize, 4);
}

enum {
	StructFieldSize = 5*PtrSize
};
// Type.StructType.fields[]-> name, typ and offset.
static char*
decodetype_structfieldname(Sym *s, int i)
{
	Reloc *r;

	// go.string."foo"  0x28 / 0x40
	s = decode_reloc_sym(s, CommonSize + PtrSize + 2*4 + i*StructFieldSize);
	if (s == nil)			// embedded structs have a nil name.
		return nil;
	r = decode_reloc(s, 0);		// s has a pointer to the string data at offset 0
	if (r == nil)			// shouldn't happen.
		return nil;
	return (char*) r->sym->p + r->add;	// the c-string
}

static Sym*
decodetype_structfieldtype(Sym *s, int i)
{
	return decode_reloc_sym(s, CommonSize + PtrSize + 2*4 + i*StructFieldSize + 2*PtrSize);
}

static vlong
decodetype_structfieldoffs(Sym *s, int i)
{
	return decode_inuxi(s->p + CommonSize + PtrSize + 2*4 + i*StructFieldSize + 4*PtrSize, 4);
}

// InterfaceTYpe.methods.len
static vlong
decodetype_ifacemethodcount(Sym *s)
{
	return decode_inuxi(s->p + CommonSize + PtrSize, 4);
}


// Fake attributes for slices, maps and channel
enum {
	DW_AT_internal_elem_type = 250,	 // channels and slices
	DW_AT_internal_key_type = 251,	 // maps
	DW_AT_internal_val_type = 252,	 // maps
	DW_AT_internal_location = 253,	 // params and locals
};

static DWDie* defptrto(DWDie *dwtype);	// below

// Lookup predefined types
static Sym*
lookup_or_diag(char *n)
{
	Sym *s;

	s = rlookup(n, 0);
	if (s == nil || s->size == 0) {
		diag("dwarf: missing type: %s", n);
		errorexit();
	}
	return s;
}

// Define gotype, for composite ones recurse into constituents.
static DWDie*
defgotype(Sym *gotype)
{
	DWDie *die, *fld;
	Sym *s;
	char *name, *f;
	uint8 kind;
	vlong bytesize;
	int i, nfields;

	if (gotype == nil)
		return find_or_diag(&dwtypes, "<unspecified>");

	if (strncmp("type.", gotype->name, 5) != 0) {
		diag("dwarf: type name doesn't start with \".type\": %s", gotype->name);
		return find_or_diag(&dwtypes, "<unspecified>");
	}
	name = gotype->name + 5;  // could also decode from Type.string

	die = find(&dwtypes, name);
	if (die != nil)
		return die;

	if (0 && debug['v'] > 2)
		print("new type: %Y\n", gotype);

	kind = decodetype_kind(gotype);
	bytesize = decodetype_size(gotype);

	switch (kind) {
	case KindBool:
		die = newdie(&dwtypes, DW_ABRV_BASETYPE, name);
		newattr(die, DW_AT_encoding,  DW_CLS_CONSTANT, DW_ATE_boolean, 0);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		break;

	case KindInt:
	case KindInt8:
	case KindInt16:
	case KindInt32:
	case KindInt64:
		die = newdie(&dwtypes, DW_ABRV_BASETYPE, name);
		newattr(die, DW_AT_encoding,  DW_CLS_CONSTANT, DW_ATE_signed, 0);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		break;

	case KindUint:
	case KindUint8:
	case KindUint16:
	case KindUint32:
	case KindUint64:
	case KindUintptr:
		die = newdie(&dwtypes, DW_ABRV_BASETYPE, name);
		newattr(die, DW_AT_encoding,  DW_CLS_CONSTANT, DW_ATE_unsigned, 0);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		break;

	case KindFloat32:
	case KindFloat64:
		die = newdie(&dwtypes, DW_ABRV_BASETYPE, name);
		newattr(die, DW_AT_encoding,  DW_CLS_CONSTANT, DW_ATE_float, 0);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		break;

	case KindComplex64:
	case KindComplex128:
		die = newdie(&dwtypes, DW_ABRV_BASETYPE, name);
		newattr(die, DW_AT_encoding,  DW_CLS_CONSTANT, DW_ATE_complex_float, 0);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		break;

	case KindArray:
		die = newdie(&dwtypes, DW_ABRV_ARRAYTYPE, name);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		s = decodetype_arrayelem(gotype);
		newrefattr(die, DW_AT_type, defgotype(s));
		fld = newdie(die, DW_ABRV_ARRAYRANGE, "range");
		newattr(fld, DW_AT_upper_bound, DW_CLS_CONSTANT, decodetype_arraylen(gotype), 0);
		newrefattr(fld, DW_AT_type, find_or_diag(&dwtypes, "uintptr"));
		break;

	case KindChan:
		die = newdie(&dwtypes, DW_ABRV_CHANTYPE, name);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		s = decodetype_chanelem(gotype);
		newrefattr(die, DW_AT_internal_elem_type, defgotype(s));
		break;

	case KindFunc:
		die = newdie(&dwtypes, DW_ABRV_FUNCTYPE, name);
		newrefattr(die, DW_AT_type, find_or_diag(&dwtypes, "void"));
		nfields = decodetype_funcincount(gotype);
		for (i = 0; i < nfields; i++) {
			s = decodetype_funcintype(gotype, i);
			fld = newdie(die, DW_ABRV_FUNCTYPEPARAM, s->name+5);
			newrefattr(fld, DW_AT_type, defgotype(s));
		}
		if (decodetype_funcdotdotdot(gotype))
			newdie(die, DW_ABRV_DOTDOTDOT, "...");
		nfields = decodetype_funcoutcount(gotype);
		for (i = 0; i < nfields; i++) {
			s = decodetype_funcouttype(gotype, i);
			fld = newdie(die, DW_ABRV_FUNCTYPEPARAM, s->name+5);
			newrefattr(fld, DW_AT_type, defptrto(defgotype(s)));
		}
		break;

	case KindInterface:
		die = newdie(&dwtypes, DW_ABRV_IFACETYPE, name);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		nfields = decodetype_ifacemethodcount(gotype);
		if (nfields == 0)
			s = lookup_or_diag("type.runtime.eface");
		else
			s = lookup_or_diag("type.runtime.iface");
		newrefattr(die, DW_AT_type, defgotype(s));
		break;

	case KindMap:
		die = newdie(&dwtypes, DW_ABRV_MAPTYPE, name);
		s = decodetype_mapkey(gotype);
		newrefattr(die, DW_AT_internal_key_type, defgotype(s));
		s = decodetype_mapvalue(gotype);
		newrefattr(die, DW_AT_internal_val_type, defgotype(s));
		break;

	case KindPtr:
		die = newdie(&dwtypes, DW_ABRV_PTRTYPE, name);
		s = decodetype_ptrelem(gotype);
		newrefattr(die, DW_AT_type, defgotype(s));
		break;

	case KindSlice:
		die = newdie(&dwtypes, DW_ABRV_SLICETYPE, name);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		s = decodetype_arrayelem(gotype);
		newrefattr(die, DW_AT_internal_elem_type, defgotype(s));
		break;

	case KindString:
		die = newdie(&dwtypes, DW_ABRV_STRINGTYPE, name);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		break;

	case KindStruct:
		die = newdie(&dwtypes, DW_ABRV_STRUCTTYPE, name);
		newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, bytesize, 0);
		nfields = decodetype_structfieldcount(gotype);
		for (i = 0; i < nfields; i++) {
			f = decodetype_structfieldname(gotype, i);
			s = decodetype_structfieldtype(gotype, i);
			if (f == nil)
				f = s->name + 5;	 // skip "type."
			fld = newdie(die, DW_ABRV_STRUCTFIELD, f);
			newrefattr(fld, DW_AT_type, defgotype(s));
			newmemberoffsetattr(fld, decodetype_structfieldoffs(gotype, i));
		}
		break;

	case KindUnsafePointer:
		die = newdie(&dwtypes, DW_ABRV_PTRTYPE, name);
		newrefattr(die, DW_AT_type, find(&dwtypes, "void"));
		break;

	default:
		diag("dwarf: definition of unknown kind %d: %s", kind, gotype->name);
		die = newdie(&dwtypes, DW_ABRV_TYPEDECL, name);
		newrefattr(die, DW_AT_type, find_or_diag(&dwtypes, "<unspecified>"));
	 }

	return die;
}

// Find or construct *T given T.
static DWDie*
defptrto(DWDie *dwtype)
{
	char ptrname[1024];
	DWDie *die;

	snprint(ptrname, sizeof ptrname, "*%s", getattr(dwtype, DW_AT_name)->data);
	die = find(&dwtypes, ptrname);
	if (die == nil) {
		die = newdie(&dwtypes, DW_ABRV_PTRTYPE,
			     strcpy(mal(strlen(ptrname)+1), ptrname));
		newrefattr(die, DW_AT_type, dwtype);
	}
	return die;
}

// Copies src's children into dst. Copies attributes by value.
// DWAttr.data is copied as pointer only.
static void
copychildren(DWDie *dst, DWDie *src)
{
	DWDie *c;
	DWAttr *a;

	for (src = src->child; src != nil; src = src->link) {
		c = newdie(dst, src->abbrev, getattr(src, DW_AT_name)->data);
		for (a = src->attr; a != nil; a = a->link)
			newattr(c, a->atr, a->cls, a->value, a->data);
		copychildren(c, src);
	}
	reverselist(&dst->child);
}

// Search children (assumed to have DW_TAG_member) for the one named
// field and set its DW_AT_type to dwtype
static void
substitutetype(DWDie *structdie, char *field, DWDie* dwtype)
{
	DWDie *child;
	DWAttr *a;

	child = find_or_diag(structdie, field);
	if (child == nil)
		return;

	a = getattr(child, DW_AT_type);
	if (a != nil)
		a->data = (char*) dwtype;
	else
		newrefattr(child, DW_AT_type, dwtype);
}

static void
synthesizestringtypes(DWDie* die)
{
	DWDie *prototype;

	prototype = defgotype(lookup_or_diag("type.runtime._string"));
	if (prototype == nil)
		return;

	for (; die != nil; die = die->link) {
		if (die->abbrev != DW_ABRV_STRINGTYPE)
			continue;
		copychildren(die, prototype);
	}
}

static void
synthesizeslicetypes(DWDie *die)
{
	DWDie *prototype, *elem;

	prototype = defgotype(lookup_or_diag("type.runtime.slice"));
	if (prototype == nil)
		return;

	for (; die != nil; die = die->link) {
		if (die->abbrev != DW_ABRV_SLICETYPE)
			continue;
		copychildren(die, prototype);
		elem = (DWDie*) getattr(die, DW_AT_internal_elem_type)->data;
		substitutetype(die, "array", defptrto(elem));
	}
}

static char*
mkinternaltypename(char *base, char *arg1, char *arg2)
{
	char buf[1024];
	char *n;

	if (arg2 == nil)
		snprint(buf, sizeof buf, "%s<%s>", base, arg1);
	else
		snprint(buf, sizeof buf, "%s<%s,%s>", base, arg1, arg2);
	n = mal(strlen(buf) + 1);
	memmove(n, buf, strlen(buf));
	return n;
}


// synthesizemaptypes is way too closely married to runtime/hashmap.c
enum {
	MaxValsize = 256 - 64
};

static void
synthesizemaptypes(DWDie *die)
{

	DWDie *hash, *hash_subtable, *hash_entry,
		*dwh, *dwhs, *dwhe, *dwhash, *keytype, *valtype, *fld;
	int hashsize, keysize, valsize, datsize, valsize_in_hash, datavo;
	DWAttr *a;

	hash		= defgotype(lookup_or_diag("type.runtime.hmap"));
	hash_subtable	= defgotype(lookup_or_diag("type.runtime.hash_subtable"));
	hash_entry	= defgotype(lookup_or_diag("type.runtime.hash_entry"));

	if (hash == nil || hash_subtable == nil || hash_entry == nil)
		return;

	dwhash = (DWDie*)getattr(find_or_diag(hash_entry, "hash"), DW_AT_type)->data;
	if (dwhash == nil)
		return;

	hashsize = getattr(dwhash, DW_AT_byte_size)->value;

	for (; die != nil; die = die->link) {
		if (die->abbrev != DW_ABRV_MAPTYPE)
			continue;

		keytype = (DWDie*) getattr(die, DW_AT_internal_key_type)->data;
		valtype = (DWDie*) getattr(die, DW_AT_internal_val_type)->data;

		a = getattr(keytype, DW_AT_byte_size);
		keysize = a ? a->value : PtrSize;  // We don't store size with Pointers

		a = getattr(valtype, DW_AT_byte_size);
		valsize = a ? a->value : PtrSize;

		// This is what happens in hash_init and makemap_c
		valsize_in_hash = valsize;
		if (valsize > MaxValsize)
			valsize_in_hash = PtrSize;
		datavo = keysize;
		if (valsize_in_hash >= PtrSize)
			datavo = rnd(keysize, PtrSize);
		datsize = datavo + valsize_in_hash;
		if (datsize < PtrSize)
			datsize = PtrSize;
		datsize = rnd(datsize, PtrSize);

		// Construct struct hash_entry<K,V>
		dwhe = newdie(&dwtypes, DW_ABRV_STRUCTTYPE,
			mkinternaltypename("hash_entry",
				getattr(keytype, DW_AT_name)->data,
				getattr(valtype, DW_AT_name)->data));

		fld = newdie(dwhe, DW_ABRV_STRUCTFIELD, "hash");
		newrefattr(fld, DW_AT_type, dwhash);
		newmemberoffsetattr(fld, 0);

		fld = newdie(dwhe, DW_ABRV_STRUCTFIELD, "key");
		newrefattr(fld, DW_AT_type, keytype);
		newmemberoffsetattr(fld, hashsize);

		fld = newdie(dwhe, DW_ABRV_STRUCTFIELD, "val");
		if (valsize > MaxValsize)
			valtype = defptrto(valtype);
		newrefattr(fld, DW_AT_type, valtype);
		newmemberoffsetattr(fld, hashsize + datavo);
		newattr(dwhe, DW_AT_byte_size, DW_CLS_CONSTANT, hashsize + datsize, nil);

		// Construct hash_subtable<hash_entry<K,V>>
		dwhs = newdie(&dwtypes, DW_ABRV_STRUCTTYPE,
			mkinternaltypename("hash_subtable",
				getattr(keytype, DW_AT_name)->data,
				getattr(valtype, DW_AT_name)->data));
		copychildren(dwhs, hash_subtable);
		substitutetype(dwhs, "last", defptrto(dwhe));
		substitutetype(dwhs, "entry", dwhe);  // todo: []hash_entry with dynamic size
		newattr(dwhs, DW_AT_byte_size, DW_CLS_CONSTANT,
			getattr(hash_subtable, DW_AT_byte_size)->value, nil);

		// Construct hash<K,V>
		dwh = newdie(&dwtypes, DW_ABRV_STRUCTTYPE,
			mkinternaltypename("hash",
				getattr(keytype, DW_AT_name)->data,
				getattr(valtype, DW_AT_name)->data));
		copychildren(dwh, hash);
		substitutetype(dwh, "st", defptrto(dwhs));
		newattr(dwh, DW_AT_byte_size, DW_CLS_CONSTANT,
			getattr(hash, DW_AT_byte_size)->value, nil);

		newrefattr(die, DW_AT_type, defptrto(dwh));
	}
}

static void
synthesizechantypes(DWDie *die)
{
	DWDie *sudog, *waitq, *hchan,
		*dws, *dww, *dwh, *elemtype;
	DWAttr *a;
	int elemsize, sudogsize;

	sudog = defgotype(lookup_or_diag("type.runtime.sudog"));
	waitq = defgotype(lookup_or_diag("type.runtime.waitq"));
	hchan = defgotype(lookup_or_diag("type.runtime.hchan"));
	if (sudog == nil || waitq == nil || hchan == nil)
		return;

	sudogsize = getattr(sudog, DW_AT_byte_size)->value;

	for (; die != nil; die = die->link) {
		if (die->abbrev != DW_ABRV_CHANTYPE)
			continue;
		elemtype = (DWDie*) getattr(die, DW_AT_internal_elem_type)->data;
		a = getattr(elemtype, DW_AT_byte_size);
		elemsize = a ? a->value : PtrSize;

		// sudog<T>
		dws = newdie(&dwtypes, DW_ABRV_STRUCTTYPE,
			mkinternaltypename("sudog",
				getattr(elemtype, DW_AT_name)->data, nil));
		copychildren(dws, sudog);
		substitutetype(dws, "elem", elemtype);
		newattr(dws, DW_AT_byte_size, DW_CLS_CONSTANT,
			sudogsize + (elemsize > 8 ? elemsize - 8 : 0), nil);

		// waitq<T>
		dww = newdie(&dwtypes, DW_ABRV_STRUCTTYPE,
			mkinternaltypename("waitq", getattr(elemtype, DW_AT_name)->data, nil));
		copychildren(dww, waitq);
		substitutetype(dww, "first", defptrto(dws));
		substitutetype(dww, "last",  defptrto(dws));
		newattr(dww, DW_AT_byte_size, DW_CLS_CONSTANT,
			getattr(waitq, DW_AT_byte_size)->value, nil);

		// hchan<T>
		dwh = newdie(&dwtypes, DW_ABRV_STRUCTTYPE,
			mkinternaltypename("hchan", getattr(elemtype, DW_AT_name)->data, nil));
		copychildren(dwh, hchan);
		substitutetype(dwh, "recvq", dww);
		substitutetype(dwh, "sendq", dww);
		newattr(dwh, DW_AT_byte_size, DW_CLS_CONSTANT,
			getattr(hchan, DW_AT_byte_size)->value, nil);

		newrefattr(die, DW_AT_type, defptrto(dwh));
	}
}

// For use with pass.c::genasmsym
static void
defdwsymb(Sym* sym, char *s, int t, vlong v, vlong size, int ver, Sym *gotype)
{
	DWDie *dv, *dt;

	USED(size);
	if (strncmp(s, "go.string.", 10) == 0)
		return;

	if (strncmp(s, "type.", 5) == 0 && strcmp(s, "type.*") != 0 && strncmp(s, "type..", 6) != 0) {
		defgotype(sym);
		return;
	}

	dv = nil;

	switch (t) {
	default:
		return;
	case 'd':
	case 'b':
	case 'D':
	case 'B':
		dv = newdie(&dwglobals, DW_ABRV_VARIABLE, s);
		newabslocexprattr(dv, v);
		if (ver == 0)
			newattr(dv, DW_AT_external, DW_CLS_FLAG, 1, 0);
		// fallthrough
	case 'a':
	case 'p':
		dt = defgotype(gotype);
	}

	if (dv != nil)
		newrefattr(dv, DW_AT_type, dt);
}

// TODO(lvd) For now, just append them all to the first compilation
// unit (that should be main), in the future distribute them to the
// appropriate compilation units.
static void
movetomodule(DWDie *parent)
{
	DWDie *die;

	for (die = dwroot.child->child; die->link != nil; die = die->link) /* nix */;
	die->link = parent->child;
}

/*
 * Filename fragments for the line history stack.
 */

static char **ftab;
static int ftabsize;

void
dwarfaddfrag(int n, char *frag)
{
	int s;

	if (n >= ftabsize) {
		s = ftabsize;
		ftabsize = 1 + n + (n >> 2);
		ftab = realloc(ftab, ftabsize * sizeof(ftab[0]));
		memset(ftab + s, 0, (ftabsize - s) * sizeof(ftab[0]));
	}

	if (*frag == '<')
		frag++;
	ftab[n] = frag;
}

// Returns a malloc'ed string, piecewise copied from the ftab.
static char *
decodez(char *s)
{
	int len, o;
	char *ss, *f;
	char *r, *rb, *re;

	len = 0;
	ss = s + 1;	// first is 0
	while((o = ((uint8)ss[0] << 8) | (uint8)ss[1]) != 0) {
		if (o < 0 || o >= ftabsize) {
			diag("dwarf: corrupt z entry");
			return 0;
		}
		f = ftab[o];
		if (f == nil) {
			diag("dwarf: corrupt z entry");
			return 0;
		}
		len += strlen(f) + 1;	// for the '/'
		ss += 2;
	}

	if (len == 0)
		return 0;

	r = malloc(len + 1);
	rb = r;
	re = rb + len + 1;

	s++;
	while((o = ((uint8)s[0] << 8) | (uint8)s[1]) != 0) {
		f = ftab[o];
		if (rb == r || rb[-1] == '/')
			rb = seprint(rb, re, "%s", f);
		else
			rb = seprint(rb, re, "/%s", f);
		s += 2;
	}
	return r;
}

/*
 * The line history itself
 */

static char **histfile;	   // [0] holds "<eof>", DW_LNS_set_file arguments must be > 0.
static int  histfilesize;
static int  histfilecap;

static void
clearhistfile(void)
{
	int i;

	// [0] holds "<eof>"
	for (i = 1; i < histfilesize; i++)
		free(histfile[i]);
	histfilesize = 0;
}

static int
addhistfile(char *zentry)
{
	char *fname;

	if (histfilesize == histfilecap) {
		histfilecap = 2 * histfilecap + 2;
		histfile = realloc(histfile, histfilecap * sizeof(char*));
	}
	if (histfilesize == 0)
		histfile[histfilesize++] = "<eof>";

	fname = decodez(zentry);
//	print("addhistfile %d: %s\n", histfilesize, fname);
	if (fname == 0)
		return -1;

	// Don't fill with duplicates (check only top one).
	if (strcmp(fname, histfile[histfilesize-1]) == 0) {
		free(fname);
		return histfilesize - 1;
	}

	histfile[histfilesize++] = fname;
	return histfilesize - 1;
}

// if the histfile stack contains ..../runtime/runtime_defs.go
// use that to set gdbscript
static void
finddebugruntimepath(void)
{
	int i, l;
	char *c;

	for (i = 1; i < histfilesize; i++) {
		if ((c = strstr(histfile[i], "runtime/zruntime_defs")) != nil) {
			l = c - histfile[i];
			memmove(gdbscript, histfile[i], l);
			memmove(gdbscript + l, "runtime/runtime-gdb.py", strlen("runtime/runtime-gdb.py") + 1);
			break;
		}
	}
}

// Go's runtime C sources are sane, and Go sources nest only 1 level,
// so a handful would be plenty, if it weren't for the fact that line
// directives can push an unlimited number of them.
static struct {
	int file;
	vlong line;
} *includestack;
static int includestacksize;
static int includetop;
static vlong absline;

typedef struct Linehist Linehist;
struct Linehist {
	Linehist *link;
	vlong absline;
	vlong line;
	int file;
};

static Linehist *linehist;

static void
checknesting(void)
{
	if (includetop < 0) {
		diag("dwarf: corrupt z stack");
		errorexit();
	}
	if (includetop >= includestacksize) {
		includestacksize += 1;
		includestacksize <<= 2;
//		print("checknesting: growing to %d\n", includestacksize);
		includestack = realloc(includestack, includestacksize * sizeof *includestack);	       
	}
}

/*
 * Return false if the a->link chain contains no history, otherwise
 * returns true and finds z and Z entries in the Auto list (of a
 * Prog), and resets the history stack
 */
static int
inithist(Auto *a)
{
	Linehist *lh;

	for (; a; a = a->link)
		if (a->type == D_FILE)
			break;
	if (a==nil)
		return 0;

	// We have a new history.  They are guaranteed to come completely
	// at the beginning of the compilation unit.
	if (a->aoffset != 1) {
		diag("dwarf: stray 'z' with offset %d", a->aoffset);
		return 0;
	}

	// Clear the history.
	clearhistfile();
	includetop = 0;
	checknesting();
	includestack[includetop].file = 0;
	includestack[includetop].line = -1;
	absline = 0;
	while (linehist != nil) {
		lh = linehist->link;
		free(linehist);
		linehist = lh;
	}

	// Construct the new one.
	for (; a; a = a->link) {
		if (a->type == D_FILE) {  // 'z'
			int f = addhistfile(a->asym->name);
			if (f < 0) {	// pop file
				includetop--;
				checknesting();
			} else {	// pushed a file (potentially same)
				includestack[includetop].line += a->aoffset - absline;
				includetop++;
				checknesting();
				includestack[includetop].file = f;
				includestack[includetop].line = 1;
			}
			absline = a->aoffset;
		} else if (a->type == D_FILE1) {  // 'Z'
			// We could just fixup the current
			// linehist->line, but there doesn't appear to
			// be a guarantee that every 'Z' is preceded
			// by its own 'z', so do the safe thing and
			// update the stack and push a new Linehist
			// entry
			includestack[includetop].line =	 a->aoffset;
		} else
			continue;
		if (linehist == 0 || linehist->absline != absline) {
			Linehist* lh = malloc(sizeof *lh);
			lh->link = linehist;
			lh->absline = absline;
			linehist = lh;
		}
		linehist->file = includestack[includetop].file;
		linehist->line = includestack[includetop].line;
	}
	return 1;
}

static Linehist *
searchhist(vlong absline)
{
	Linehist *lh;

	for (lh = linehist; lh; lh = lh->link)
		if (lh->absline <= absline)
			break;
	return lh;
}

static int
guesslang(char *s)
{
	if(strlen(s) >= 3 && strcmp(s+strlen(s)-3, ".go") == 0)
		return DW_LANG_Go;

	return DW_LANG_C;
}

/*
 * Generate short opcodes when possible, long ones when neccesary.
 * See section 6.2.5
 */

enum {
	LINE_BASE = -1,
	LINE_RANGE = 4,
	OPCODE_BASE = 5
};

static void
putpclcdelta(vlong delta_pc, vlong delta_lc)
{
	if (LINE_BASE <= delta_lc && delta_lc < LINE_BASE+LINE_RANGE) {
		vlong opcode = OPCODE_BASE + (delta_lc - LINE_BASE) + (LINE_RANGE * delta_pc);
		if (OPCODE_BASE <= opcode && opcode < 256) {
			cput(opcode);
			return;
		}
	}

	if (delta_pc) {
		cput(DW_LNS_advance_pc);
		sleb128put(delta_pc);
	}

	cput(DW_LNS_advance_line);
	sleb128put(delta_lc);
	cput(DW_LNS_copy);
}

static void
newcfaoffsetattr(DWDie *die, int32 offs)
{
	char block[10];
	int i;

	i = 0;

	block[i++] = DW_OP_call_frame_cfa;
	if (offs != 0) {
		block[i++] = DW_OP_consts;
		i += sleb128enc(offs, block+i);
		block[i++] = DW_OP_plus;
	}
	newattr(die, DW_AT_location, DW_CLS_BLOCK, i, mal(i));
	memmove(die->attr->data, block, i);
}

static char*
mkvarname(char* name, int da)
{
	char buf[1024];
	char *n;

	snprint(buf, sizeof buf, "%s#%d", name, da);
	n = mal(strlen(buf) + 1);
	memmove(n, buf, strlen(buf));
	return n;
}

/*
 * Walk prog table, emit line program and build DIE tree.
 */

// flush previous compilation unit.
static void
flushunit(DWDie *dwinfo, vlong pc, vlong unitstart, int32 header_length)
{
	vlong here;

	if (dwinfo != nil && pc != 0) {
		newattr(dwinfo, DW_AT_high_pc, DW_CLS_ADDRESS, pc+1, 0);
	}

	if (unitstart >= 0) {
		cput(0);  // start extended opcode
		uleb128put(1);
		cput(DW_LNE_end_sequence);

		here = cpos();
		cseek(unitstart);
		LPUT(here - unitstart - sizeof(int32));	 // unit_length
		WPUT(3);  // dwarf version
		LPUT(header_length); // header length starting here
		cseek(here);
	}
}

static void
writelines(void)
{
	Prog *q;
	Sym *s;
	Auto *a;
	vlong unitstart, headerend, offs;
	vlong pc, epc, lc, llc, lline;
	int currfile;
	int i, lang, da, dt;
	Linehist *lh;
	DWDie *dwinfo, *dwfunc, *dwvar, **dws;
	DWDie *varhash[HASHSIZE];
	char *n, *nn;

	unitstart = -1;
	headerend = -1;
	pc = 0;
	epc = 0;
	lc = 1;
	llc = 1;
	currfile = -1;
	lineo = cpos();
	dwinfo = nil;

	for(cursym = textp; cursym != nil; cursym = cursym->next) {
		s = cursym;
		if(s->text == P)
			continue;

		// Look for history stack.  If we find one,
		// we're entering a new compilation unit

		if (inithist(s->autom)) {
			flushunit(dwinfo, epc, unitstart, headerend - unitstart - 10);
			unitstart = cpos();

			if(debug['v'] > 1) {
				print("dwarf writelines found %s\n", histfile[1]);
				Linehist* lh;
				for (lh = linehist; lh; lh = lh->link)
					print("\t%8lld: [%4lld]%s\n",
					      lh->absline, lh->line, histfile[lh->file]);
			}

			lang = guesslang(histfile[1]);
			finddebugruntimepath();

			dwinfo = newdie(&dwroot, DW_ABRV_COMPUNIT, strdup(histfile[1]));
			newattr(dwinfo, DW_AT_language, DW_CLS_CONSTANT,lang, 0);
			newattr(dwinfo, DW_AT_stmt_list, DW_CLS_PTR, unitstart - lineo, 0);
			newattr(dwinfo, DW_AT_low_pc, DW_CLS_ADDRESS, s->text->pc, 0);

			// Write .debug_line Line Number Program Header (sec 6.2.4)
			// Fields marked with (*) must be changed for 64-bit dwarf
			LPUT(0);   // unit_length (*), will be filled in by flushunit.
			WPUT(3);   // dwarf version (appendix F)
			LPUT(0);   // header_length (*), filled in by flushunit.
			// cpos == unitstart + 4 + 2 + 4
			cput(1);   // minimum_instruction_length
			cput(1);   // default_is_stmt
			cput(LINE_BASE);     // line_base
			cput(LINE_RANGE);    // line_range
			cput(OPCODE_BASE);   // opcode_base (we only use 1..4)
			cput(0);   // standard_opcode_lengths[1]
			cput(1);   // standard_opcode_lengths[2]
			cput(1);   // standard_opcode_lengths[3]
			cput(1);   // standard_opcode_lengths[4]
			cput(0);   // include_directories  (empty)

			for (i=1; i < histfilesize; i++) {
				strnput(histfile[i], strlen(histfile[i]) + 4);
				// 4 zeros: the string termination + 3 fields.
			}

			cput(0);   // terminate file_names.
			headerend = cpos();

			pc = s->text->pc;
			epc = pc;
			currfile = 1;
			lc = 1;
			llc = 1;

			cput(0);  // start extended opcode
			uleb128put(1 + PtrSize);
			cput(DW_LNE_set_address);
			addrput(pc);
		}
		if(s->text == nil)
			continue;

		if (unitstart < 0) {
			diag("dwarf: reachable code before seeing any history: %P", s->text);
			continue;
		}

		dwfunc = newdie(dwinfo, DW_ABRV_FUNCTION, s->name);
		newattr(dwfunc, DW_AT_low_pc, DW_CLS_ADDRESS, s->value, 0);
		epc = s->value + s->size;
		newattr(dwfunc, DW_AT_high_pc, DW_CLS_ADDRESS, epc, 0);
		if (s->version == 0)
			newattr(dwfunc, DW_AT_external, DW_CLS_FLAG, 1, 0);

		if(s->text->link == nil)
			continue;

		for(q = s->text; q != P; q = q->link) {
			lh = searchhist(q->line);
			if (lh == nil) {
				diag("dwarf: corrupt history or bad absolute line: %P", q);
				continue;
			}

			if (lh->file < 1) {  // 0 is the past-EOF entry.
				// diag("instruction with line number past EOF in %s: %P", histfile[1], q);
				continue;
			}

			lline = lh->line + q->line - lh->absline;
			if (debug['v'] > 1)
				print("%6llux %s[%lld] %P\n", (vlong)q->pc, histfile[lh->file], lline, q);

			if (q->line == lc)
				continue;
			if (currfile != lh->file) {
				currfile = lh->file;
				cput(DW_LNS_set_file);
				uleb128put(currfile);
			}
			putpclcdelta(q->pc - pc, lline - llc);
			pc  = q->pc;
			lc  = q->line;
			llc = lline;
		}

		da = 0;
		dwfunc->hash = varhash;	 // enable indexing of children by name
		memset(varhash, 0, sizeof varhash);
		for(a = s->autom; a; a = a->link) {
			switch (a->type) {
			case D_AUTO:
				dt = DW_ABRV_AUTO;
				offs = a->aoffset - PtrSize;
				break;
			case D_PARAM:
				dt = DW_ABRV_PARAM;
				offs = a->aoffset;
				break;
			default:
				continue;
			}
			if (strstr(a->asym->name, ".autotmp_"))
				continue;
			if (find(dwfunc, a->asym->name) != nil)
				n = mkvarname(a->asym->name, da);
			else
				n = a->asym->name;
			// Drop the package prefix from locals and arguments.
			nn = strrchr(n, '.');
			if (nn)
				n = nn + 1;

			dwvar = newdie(dwfunc, dt, n);
			newcfaoffsetattr(dwvar, offs);
			newrefattr(dwvar, DW_AT_type, defgotype(a->gotype));

			// push dwvar down dwfunc->child to preserve order
			newattr(dwvar, DW_AT_internal_location, DW_CLS_CONSTANT, offs, nil);
			dwfunc->child = dwvar->link;  // take dwvar out from the top of the list
			for (dws = &dwfunc->child; *dws != nil; dws = &(*dws)->link)
				if (offs > getattr(*dws, DW_AT_internal_location)->value)
					break;
			dwvar->link = *dws;
			*dws = dwvar;

			da++;
		}

		dwfunc->hash = nil;
	}

	flushunit(dwinfo, epc, unitstart, headerend - unitstart - 10);
	linesize = cpos() - lineo;
}

/*
 *  Emit .debug_frame
 */
enum
{
	CIERESERVE = 16,
	DATAALIGNMENTFACTOR = -4,	// TODO -PtrSize?
	FAKERETURNCOLUMN = 16		// TODO gdb6 doesnt like > 15?
};

static void
putpccfadelta(vlong deltapc, vlong cfa)
{
	if (deltapc < 0x40) {
		cput(DW_CFA_advance_loc + deltapc);
	} else if (deltapc < 0x100) {
		cput(DW_CFA_advance_loc1);
		cput(deltapc);
	} else if (deltapc < 0x10000) {
		cput(DW_CFA_advance_loc2);
		WPUT(deltapc);
	} else {
		cput(DW_CFA_advance_loc4);
		LPUT(deltapc);
	}

	cput(DW_CFA_def_cfa_offset_sf);
	sleb128put(cfa / DATAALIGNMENTFACTOR);
}

static void
writeframes(void)
{
	Prog *p, *q;
	Sym *s;
	vlong fdeo, fdesize, pad, cfa, pc;

	frameo = cpos();

	// Emit the CIE, Section 6.4.1
	LPUT(CIERESERVE);	// initial length, must be multiple of PtrSize
	LPUT(0xffffffff);	// cid.
	cput(3);		// dwarf version (appendix F)
	cput(0);		// augmentation ""
	uleb128put(1);		// code_alignment_factor
	sleb128put(DATAALIGNMENTFACTOR); // guess
	uleb128put(FAKERETURNCOLUMN);	// return_address_register

	cput(DW_CFA_def_cfa);
	uleb128put(DWARFREGSP);	// register SP (**ABI-dependent, defined in l.h)
	uleb128put(PtrSize);	// offset

	cput(DW_CFA_offset + FAKERETURNCOLUMN);	 // return address
	uleb128put(-PtrSize / DATAALIGNMENTFACTOR);  // at cfa - x*4

	// 4 is to exclude the length field.
	pad = CIERESERVE + frameo + 4 - cpos();
	if (pad < 0) {
		diag("dwarf: CIERESERVE too small by %lld bytes.", -pad);
		errorexit();
	}
	strnput("", pad);

	for(cursym = textp; cursym != nil; cursym = cursym->next) {
		s = cursym;
		if(s->text == nil)
			continue;

		fdeo = cpos();
		// Emit a FDE, Section 6.4.1, starting wit a placeholder.
		LPUT(0);	// length, must be multiple of PtrSize
		LPUT(0);	// Pointer to the CIE above, at offset 0
		addrput(0);	// initial location
		addrput(0);	// address range

		cfa = PtrSize;	// CFA starts at sp+PtrSize
		p = s->text;
		pc = p->pc;

		for(q = p; q->link != P; q = q->link) {
			if (q->spadj == 0)
				continue;
			cfa += q->spadj;
			putpccfadelta(q->link->pc - pc, cfa);
			pc = q->link->pc;
		}

		fdesize = cpos() - fdeo - 4;	// exclude the length field.
		pad = rnd(fdesize, PtrSize) - fdesize;
		strnput("", pad);
		fdesize += pad;

		// Emit the FDE header for real, Section 6.4.1.
		cseek(fdeo);
		LPUT(fdesize);
		LPUT(0);
		addrput(p->pc);
		addrput(s->size);
		cseek(fdeo + 4 + fdesize);
	}

	cflush();
	framesize = cpos() - frameo;
}

/*
 *  Walk DWarfDebugInfoEntries, and emit .debug_info
 */
enum
{
	COMPUNITHEADERSIZE = 4+2+4+1
};

static void
writeinfo(void)
{
	DWDie *compunit;
	vlong unitstart, here;

	fwdcount = 0;

	for (compunit = dwroot.child; compunit; compunit = compunit->link) {
		unitstart = cpos();

		// Write .debug_info Compilation Unit Header (sec 7.5.1)
		// Fields marked with (*) must be changed for 64-bit dwarf
		// This must match COMPUNITHEADERSIZE above.
		LPUT(0);	// unit_length (*), will be filled in later.
		WPUT(3);	// dwarf version (appendix F)
		LPUT(0);	// debug_abbrev_offset (*)
		cput(PtrSize);	// address_size

		putdie(compunit);

		here = cpos();
		cseek(unitstart);
		LPUT(here - unitstart - 4);	// exclude the length field.
		cseek(here);
	}
	cflush();
}

/*
 *  Emit .debug_pubnames/_types.  _info must have been written before,
 *  because we need die->offs and infoo/infosize;
 */
static int
ispubname(DWDie *die) {
	DWAttr *a;

	switch(die->abbrev) {
	case DW_ABRV_FUNCTION:
	case DW_ABRV_VARIABLE:
		a = getattr(die, DW_AT_external);
		return a && a->value;
	}
	return 0;
}

static int
ispubtype(DWDie *die) {
	return die->abbrev >= DW_ABRV_NULLTYPE;
}

static vlong
writepub(int (*ispub)(DWDie*))
{
	DWDie *compunit, *die;
	DWAttr *dwa;
	vlong unitstart, unitend, sectionstart, here;

	sectionstart = cpos();

	for (compunit = dwroot.child; compunit != nil; compunit = compunit->link) {
		unitstart = compunit->offs - COMPUNITHEADERSIZE;
		if (compunit->link != nil)
			unitend = compunit->link->offs - COMPUNITHEADERSIZE;
		else
			unitend = infoo + infosize;

		// Write .debug_pubnames/types	Header (sec 6.1.1)
		LPUT(0);			// unit_length (*), will be filled in later.
		WPUT(2);			// dwarf version (appendix F)
		LPUT(unitstart);		// debug_info_offset (of the Comp unit Header)
		LPUT(unitend - unitstart);	// debug_info_length

		for (die = compunit->child; die != nil; die = die->link) {
			if (!ispub(die)) continue;
			LPUT(die->offs - unitstart);
			dwa = getattr(die, DW_AT_name);
			strnput(dwa->data, dwa->value + 1);
		}
		LPUT(0);

		here = cpos();
		cseek(sectionstart);
		LPUT(here - sectionstart - 4);	// exclude the length field.
		cseek(here);

	}

	return sectionstart;
}

/*
 *  emit .debug_aranges.  _info must have been written before,
 *  because we need die->offs of dw_globals.
 */
static vlong
writearanges(void)
{
	DWDie *compunit;
	DWAttr *b, *e;
	int headersize;
	vlong sectionstart;

	sectionstart = cpos();
	headersize = rnd(4+2+4+1+1, PtrSize);  // don't count unit_length field itself

	for (compunit = dwroot.child; compunit != nil; compunit = compunit->link) {
		b = getattr(compunit,  DW_AT_low_pc);
		if (b == nil)
			continue;
		e = getattr(compunit,  DW_AT_high_pc);
		if (e == nil)
			continue;

		// Write .debug_aranges	 Header + entry	 (sec 6.1.2)
		LPUT(headersize + 4*PtrSize - 4);	// unit_length (*)
		WPUT(2);	// dwarf version (appendix F)
		LPUT(compunit->offs - COMPUNITHEADERSIZE);	// debug_info_offset
		cput(PtrSize);	// address_size
		cput(0);	// segment_size
		strnput("", headersize - (4+2+4+1+1));	// align to PtrSize

		addrput(b->value);
		addrput(e->value - b->value);
		addrput(0);
		addrput(0);
	}
	cflush();
	return sectionstart;
}

static vlong
writegdbscript(void)
{
	vlong sectionstart;

	sectionstart = cpos();

	if (gdbscript[0]) {
		cput(1);  // magic 1 byte?
		strnput(gdbscript, strlen(gdbscript)+1);
		cflush();
	}
	return sectionstart;
}

static void
align(vlong size)
{
	if(HEADTYPE == Hwindows) // Only Windows PE need section align.
		strnput("", rnd(size, PEFILEALIGN) - size);
}

/*
 * This is the main entry point for generating dwarf.  After emitting
 * the mandatory debug_abbrev section, it calls writelines() to set up
 * the per-compilation unit part of the DIE tree, while simultaneously
 * emitting the debug_line section.  When the final tree contains
 * forward references, it will write the debug_info section in 2
 * passes.
 *
 */
void
dwarfemitdebugsections(void)
{
	vlong infoe;
	DWDie* die;

	if(debug['w'])  // disable dwarf
		return;

	// For diagnostic messages.
	newattr(&dwtypes, DW_AT_name, DW_CLS_STRING, strlen("dwtypes"), "dwtypes");

	mkindex(&dwroot);
	mkindex(&dwtypes);
	mkindex(&dwglobals);

	// Some types that must exist to define other ones.
	newdie(&dwtypes, DW_ABRV_NULLTYPE, "<unspecified>");
	newdie(&dwtypes, DW_ABRV_NULLTYPE, "void");
	newrefattr(newdie(&dwtypes, DW_ABRV_PTRTYPE, "unsafe.Pointer"),
		DW_AT_type, find(&dwtypes, "void"));
	die = newdie(&dwtypes, DW_ABRV_BASETYPE, "uintptr");  // needed for array size
	newattr(die, DW_AT_encoding,  DW_CLS_CONSTANT, DW_ATE_unsigned, 0);
	newattr(die, DW_AT_byte_size, DW_CLS_CONSTANT, PtrSize, 0);

	// Needed by the prettyprinter code for interface inspection.
	defgotype(lookup_or_diag("type.runtime.commonType"));
	defgotype(lookup_or_diag("type.runtime.interfaceType"));
	defgotype(lookup_or_diag("type.runtime.itab"));

	genasmsym(defdwsymb);

	writeabbrev();
	align(abbrevsize);
	writelines();
	align(linesize);
	writeframes();
	align(framesize);

	synthesizestringtypes(dwtypes.child);
	synthesizeslicetypes(dwtypes.child);
	synthesizemaptypes(dwtypes.child);
	synthesizechantypes(dwtypes.child);

	reversetree(&dwroot.child);
	reversetree(&dwtypes.child);
	reversetree(&dwglobals.child);

	movetomodule(&dwtypes);
	movetomodule(&dwglobals);

	infoo = cpos();
	writeinfo();
	infoe = cpos();
	pubnameso = infoe;
	pubtypeso = infoe;
	arangeso = infoe;
	gdbscripto = infoe;

	if (fwdcount > 0) {
		if (debug['v'])
			Bprint(&bso, "%5.2f dwarf pass 2.\n", cputime());
		cseek(infoo);
		writeinfo();
		if (fwdcount > 0) {
			diag("dwarf: unresolved references after first dwarf info pass");
			errorexit();
		}
		if (infoe != cpos()) {
			diag("dwarf: inconsistent second dwarf info pass");
			errorexit();
		}
	}
	infosize = infoe - infoo;
	align(infosize);

	pubnameso  = writepub(ispubname);
	pubnamessize  = cpos() - pubnameso;
	align(pubnamessize);

	pubtypeso  = writepub(ispubtype);
	pubtypessize  = cpos() - pubtypeso;
	align(pubtypessize);

	arangeso   = writearanges();
	arangessize   = cpos() - arangeso;
	align(arangessize);

	gdbscripto = writegdbscript();
	gdbscriptsize = cpos() - gdbscripto;
	align(gdbscriptsize);
}

/*
 *  Elf.
 */
enum
{
	ElfStrDebugAbbrev,
	ElfStrDebugAranges,
	ElfStrDebugFrame,
	ElfStrDebugInfo,
	ElfStrDebugLine,
	ElfStrDebugLoc,
	ElfStrDebugMacinfo,
	ElfStrDebugPubNames,
	ElfStrDebugPubTypes,
	ElfStrDebugRanges,
	ElfStrDebugStr,
	ElfStrGDBScripts,
	NElfStrDbg
};

vlong elfstrdbg[NElfStrDbg];

void
dwarfaddshstrings(Sym *shstrtab)
{
	if(debug['w'])  // disable dwarf
		return;

	elfstrdbg[ElfStrDebugAbbrev]   = addstring(shstrtab, ".debug_abbrev");
	elfstrdbg[ElfStrDebugAranges]  = addstring(shstrtab, ".debug_aranges");
	elfstrdbg[ElfStrDebugFrame]    = addstring(shstrtab, ".debug_frame");
	elfstrdbg[ElfStrDebugInfo]     = addstring(shstrtab, ".debug_info");
	elfstrdbg[ElfStrDebugLine]     = addstring(shstrtab, ".debug_line");
	elfstrdbg[ElfStrDebugLoc]      = addstring(shstrtab, ".debug_loc");
	elfstrdbg[ElfStrDebugMacinfo]  = addstring(shstrtab, ".debug_macinfo");
	elfstrdbg[ElfStrDebugPubNames] = addstring(shstrtab, ".debug_pubnames");
	elfstrdbg[ElfStrDebugPubTypes] = addstring(shstrtab, ".debug_pubtypes");
	elfstrdbg[ElfStrDebugRanges]   = addstring(shstrtab, ".debug_ranges");
	elfstrdbg[ElfStrDebugStr]      = addstring(shstrtab, ".debug_str");
	elfstrdbg[ElfStrGDBScripts]    = addstring(shstrtab, ".debug_gdb_scripts");
}

void
dwarfaddelfheaders(void)
{
	ElfShdr *sh;

	if(debug['w'])  // disable dwarf
		return;

	sh = newElfShdr(elfstrdbg[ElfStrDebugAbbrev]);
	sh->type = SHT_PROGBITS;
	sh->off = abbrevo;
	sh->size = abbrevsize;
	sh->addralign = 1;

	sh = newElfShdr(elfstrdbg[ElfStrDebugLine]);
	sh->type = SHT_PROGBITS;
	sh->off = lineo;
	sh->size = linesize;
	sh->addralign = 1;

	sh = newElfShdr(elfstrdbg[ElfStrDebugFrame]);
	sh->type = SHT_PROGBITS;
	sh->off = frameo;
	sh->size = framesize;
	sh->addralign = 1;

	sh = newElfShdr(elfstrdbg[ElfStrDebugInfo]);
	sh->type = SHT_PROGBITS;
	sh->off = infoo;
	sh->size = infosize;
	sh->addralign = 1;

	if (pubnamessize > 0) {
		sh = newElfShdr(elfstrdbg[ElfStrDebugPubNames]);
		sh->type = SHT_PROGBITS;
		sh->off = pubnameso;
		sh->size = pubnamessize;
		sh->addralign = 1;
	}

	if (pubtypessize > 0) {
		sh = newElfShdr(elfstrdbg[ElfStrDebugPubTypes]);
		sh->type = SHT_PROGBITS;
		sh->off = pubtypeso;
		sh->size = pubtypessize;
		sh->addralign = 1;
	}

	if (arangessize) {
		sh = newElfShdr(elfstrdbg[ElfStrDebugAranges]);
		sh->type = SHT_PROGBITS;
		sh->off = arangeso;
		sh->size = arangessize;
		sh->addralign = 1;
	}

	if (gdbscriptsize) {
		sh = newElfShdr(elfstrdbg[ElfStrGDBScripts]);
		sh->type = SHT_PROGBITS;
		sh->off = gdbscripto;
		sh->size = gdbscriptsize;
		sh->addralign = 1;
	}
}

/*
 * Macho
 */
void
dwarfaddmachoheaders(void)
{
	MachoSect *msect;
	MachoSeg *ms;
	vlong fakestart;
	int nsect;

	if(debug['w'])  // disable dwarf
		return;

	// Zero vsize segments won't be loaded in memory, even so they
	// have to be page aligned in the file.
	fakestart = abbrevo & ~0xfff;

	nsect = 4;
	if (pubnamessize  > 0)
		nsect++;
	if (pubtypessize  > 0)
		nsect++;
	if (arangessize	  > 0)
		nsect++;
	if (gdbscriptsize > 0)
		nsect++;

	ms = newMachoSeg("__DWARF", nsect);
	ms->fileoffset = fakestart;
	ms->filesize = abbrevo-fakestart;

	msect = newMachoSect(ms, "__debug_abbrev");
	msect->off = abbrevo;
	msect->size = abbrevsize;
	ms->filesize += msect->size;

	msect = newMachoSect(ms, "__debug_line");
	msect->off = lineo;
	msect->size = linesize;
	ms->filesize += msect->size;

	msect = newMachoSect(ms, "__debug_frame");
	msect->off = frameo;
	msect->size = framesize;
	ms->filesize += msect->size;

	msect = newMachoSect(ms, "__debug_info");
	msect->off = infoo;
	msect->size = infosize;
	ms->filesize += msect->size;

	if (pubnamessize > 0) {
		msect = newMachoSect(ms, "__debug_pubnames");
		msect->off = pubnameso;
		msect->size = pubnamessize;
		ms->filesize += msect->size;
	}

	if (pubtypessize > 0) {
		msect = newMachoSect(ms, "__debug_pubtypes");
		msect->off = pubtypeso;
		msect->size = pubtypessize;
		ms->filesize += msect->size;
	}

	if (arangessize > 0) {
		msect = newMachoSect(ms, "__debug_aranges");
		msect->off = arangeso;
		msect->size = arangessize;
		ms->filesize += msect->size;
	}

	// TODO(lvd) fix gdb/python to load MachO (16 char section name limit)
	if (gdbscriptsize > 0) {
		msect = newMachoSect(ms, "__debug_gdb_scripts");
		msect->off = gdbscripto;
		msect->size = gdbscriptsize;
		ms->filesize += msect->size;
	}
}

/*
 * Windows PE
 */
void
dwarfaddpeheaders(void)
{
	if(debug['w'])  // disable dwarf
		return;

	newPEDWARFSection(".debug_abbrev", abbrevsize);
	newPEDWARFSection(".debug_line", linesize);
	newPEDWARFSection(".debug_frame", framesize);
	newPEDWARFSection(".debug_info", infosize);
	newPEDWARFSection(".debug_pubnames", pubnamessize);
	newPEDWARFSection(".debug_pubtypes", pubtypessize);
	newPEDWARFSection(".debug_aranges", arangessize);
	newPEDWARFSection(".debug_gdb_scripts", gdbscriptsize);
}
