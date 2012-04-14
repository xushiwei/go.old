// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"strings"
)

var conf = printer.Config{Mode: printer.SourcePos, Tabwidth: 8}

// writeDefs creates output files to be compiled by 6g, 6c, and gcc.
// (The comments here say 6g and 6c but the code applies to the 8 and 5 tools too.)
func (p *Package) writeDefs() {
	fgo2 := creat(*objDir + "_cgo_gotypes.go")
	fc := creat(*objDir + "_cgo_defun.c")
	fm := creat(*objDir + "_cgo_main.c")

	fflg := creat(*objDir + "_cgo_flags")
	for k, v := range p.CgoFlags {
		fmt.Fprintf(fflg, "_CGO_%s=%s\n", k, v)
	}
	fflg.Close()

	// Write C main file for using gcc to resolve imports.
	fmt.Fprintf(fm, "int main() { return 0; }\n")
	if *importRuntimeCgo {
		fmt.Fprintf(fm, "void crosscall2(void(*fn)(void*, int), void *a, int c) { }\n")
	} else {
		// If we're not importing runtime/cgo, we *are* runtime/cgo,
		// which provides crosscall2.  We just need a prototype.
		fmt.Fprintf(fm, "void crosscall2(void(*fn)(void*, int), void *a, int c);")
	}
	fmt.Fprintf(fm, "void _cgo_allocate(void *a, int c) { }\n")
	fmt.Fprintf(fm, "void _cgo_panic(void *a, int c) { }\n")

	// Write second Go output: definitions of _C_xxx.
	// In a separate file so that the import of "unsafe" does not
	// pollute the original file.
	fmt.Fprintf(fgo2, "// Created by cgo - DO NOT EDIT\n\n")
	fmt.Fprintf(fgo2, "package %s\n\n", p.PackageName)
	fmt.Fprintf(fgo2, "import \"unsafe\"\n\n")
	fmt.Fprintf(fgo2, "import \"syscall\"\n\n")
	if !*gccgo && *importRuntimeCgo {
		fmt.Fprintf(fgo2, "import _ \"runtime/cgo\"\n\n")
	}
	fmt.Fprintf(fgo2, "type _ unsafe.Pointer\n\n")
	fmt.Fprintf(fgo2, "func _Cerrno(dst *error, x int) { *dst = syscall.Errno(x) }\n")

	for name, def := range typedef {
		fmt.Fprintf(fgo2, "type %s ", name)
		conf.Fprint(fgo2, fset, def.Go)
		fmt.Fprintf(fgo2, "\n\n")
	}
	fmt.Fprintf(fgo2, "type _Ctype_void [0]byte\n")

	if *gccgo {
		fmt.Fprintf(fc, cPrologGccgo)
	} else {
		fmt.Fprintf(fc, cProlog)
	}

	cVars := make(map[string]bool)
	for _, n := range p.Name {
		if n.Kind != "var" {
			continue
		}

		if !cVars[n.C] {
			fmt.Fprintf(fm, "extern char %s[];\n", n.C)
			fmt.Fprintf(fm, "void *_cgohack_%s = %s;\n\n", n.C, n.C)

			fmt.Fprintf(fc, "extern byte *%s;\n", n.C)

			cVars[n.C] = true
		}

		fmt.Fprintf(fc, "void *·%s = &%s;\n", n.Mangle, n.C)
		fmt.Fprintf(fc, "\n")

		fmt.Fprintf(fgo2, "var %s ", n.Mangle)
		conf.Fprint(fgo2, fset, &ast.StarExpr{X: n.Type.Go})
		fmt.Fprintf(fgo2, "\n")
	}
	fmt.Fprintf(fc, "\n")

	for _, n := range p.Name {
		if n.Const != "" {
			fmt.Fprintf(fgo2, "const _Cconst_%s = %s\n", n.Go, n.Const)
		}
	}
	fmt.Fprintf(fgo2, "\n")

	for _, n := range p.Name {
		if n.FuncType != nil {
			p.writeDefsFunc(fc, fgo2, n)
		}
	}

	if *gccgo {
		p.writeGccgoExports(fgo2, fc, fm)
	} else {
		p.writeExports(fgo2, fc, fm)
	}

	fgo2.Close()
	fc.Close()
}

func dynimport(obj string) {
	stdout := os.Stdout
	if *dynout != "" {
		f, err := os.Create(*dynout)
		if err != nil {
			fatalf("%s", err)
		}
		stdout = f
	}

	if f, err := elf.Open(obj); err == nil {
		sym, err := f.ImportedSymbols()
		if err != nil {
			fatalf("cannot load imported symbols from ELF file %s: %v", obj, err)
		}
		for _, s := range sym {
			targ := s.Name
			if s.Version != "" {
				targ += "@" + s.Version
			}
			fmt.Fprintf(stdout, "#pragma dynimport %s %s %q\n", s.Name, targ, s.Library)
		}
		lib, err := f.ImportedLibraries()
		if err != nil {
			fatalf("cannot load imported libraries from ELF file %s: %v", obj, err)
		}
		for _, l := range lib {
			fmt.Fprintf(stdout, "#pragma dynimport _ _ %q\n", l)
		}
		return
	}

	if f, err := macho.Open(obj); err == nil {
		sym, err := f.ImportedSymbols()
		if err != nil {
			fatalf("cannot load imported symbols from Mach-O file %s: %v", obj, err)
		}
		for _, s := range sym {
			if len(s) > 0 && s[0] == '_' {
				s = s[1:]
			}
			fmt.Fprintf(stdout, "#pragma dynimport %s %s %q\n", s, s, "")
		}
		lib, err := f.ImportedLibraries()
		if err != nil {
			fatalf("cannot load imported libraries from Mach-O file %s: %v", obj, err)
		}
		for _, l := range lib {
			fmt.Fprintf(stdout, "#pragma dynimport _ _ %q\n", l)
		}
		return
	}

	if f, err := pe.Open(obj); err == nil {
		sym, err := f.ImportedSymbols()
		if err != nil {
			fatalf("cannot load imported symbols from PE file %s: %v", obj, err)
		}
		for _, s := range sym {
			ss := strings.Split(s, ":")
			fmt.Fprintf(stdout, "#pragma dynimport %s %s %q\n", ss[0], ss[0], strings.ToLower(ss[1]))
		}
		return
	}

	fatalf("cannot parse %s as ELF, Mach-O or PE", obj)
}

// Construct a gcc struct matching the 6c argument frame.
// Assumes that in gcc, char is 1 byte, short 2 bytes, int 4 bytes, long long 8 bytes.
// These assumptions are checked by the gccProlog.
// Also assumes that 6c convention is to word-align the
// input and output parameters.
func (p *Package) structType(n *Name) (string, int64) {
	var buf bytes.Buffer
	fmt.Fprint(&buf, "struct {\n")
	off := int64(0)
	for i, t := range n.FuncType.Params {
		if off%t.Align != 0 {
			pad := t.Align - off%t.Align
			fmt.Fprintf(&buf, "\t\tchar __pad%d[%d];\n", off, pad)
			off += pad
		}
		c := t.Typedef
		if c == "" {
			c = t.C.String()
		}
		fmt.Fprintf(&buf, "\t\t%s p%d;\n", c, i)
		off += t.Size
	}
	if off%p.PtrSize != 0 {
		pad := p.PtrSize - off%p.PtrSize
		fmt.Fprintf(&buf, "\t\tchar __pad%d[%d];\n", off, pad)
		off += pad
	}
	if t := n.FuncType.Result; t != nil {
		if off%t.Align != 0 {
			pad := t.Align - off%t.Align
			fmt.Fprintf(&buf, "\t\tchar __pad%d[%d];\n", off, pad)
			off += pad
		}
		qual := ""
		if c := t.C.String(); c[len(c)-1] == '*' {
			qual = "const "
		}
		fmt.Fprintf(&buf, "\t\t%s%s r;\n", qual, t.C)
		off += t.Size
	}
	if off%p.PtrSize != 0 {
		pad := p.PtrSize - off%p.PtrSize
		fmt.Fprintf(&buf, "\t\tchar __pad%d[%d];\n", off, pad)
		off += pad
	}
	if n.AddError {
		fmt.Fprint(&buf, "\t\tvoid *e[2]; /* error */\n")
		off += 2 * p.PtrSize
	}
	if off == 0 {
		fmt.Fprintf(&buf, "\t\tchar unused;\n") // avoid empty struct
	}
	fmt.Fprintf(&buf, "\t}")
	return buf.String(), off
}

func (p *Package) writeDefsFunc(fc, fgo2 *os.File, n *Name) {
	name := n.Go
	gtype := n.FuncType.Go
	if n.AddError {
		// Add "error" to return type list.
		// Type list is known to be 0 or 1 element - it's a C function.
		err := &ast.Field{Type: ast.NewIdent("error")}
		l := gtype.Results.List
		if len(l) == 0 {
			l = []*ast.Field{err}
		} else {
			l = []*ast.Field{l[0], err}
		}
		t := new(ast.FuncType)
		*t = *gtype
		t.Results = &ast.FieldList{List: l}
		gtype = t
	}

	// Go func declaration.
	d := &ast.FuncDecl{
		Name: ast.NewIdent(n.Mangle),
		Type: gtype,
	}

	if *gccgo {
		// Gccgo style hooks.
		// we hook directly into C. gccgo goes not support cgocall yet.
		if !n.AddError {
			fmt.Fprintf(fgo2, "//extern %s\n", n.C)
			conf.Fprint(fgo2, fset, d)
			fmt.Fprint(fgo2, "\n")
		} else {
			// write a small wrapper to retrieve errno.
			cname := fmt.Sprintf("_cgo%s%s", cPrefix, n.Mangle)
			paramnames := []string(nil)
			for i, param := range d.Type.Params.List {
				paramName := fmt.Sprintf("p%d", i)
				param.Names = []*ast.Ident{ast.NewIdent(paramName)}
				paramnames = append(paramnames, paramName)
			}
			conf.Fprint(fgo2, fset, d)
			fmt.Fprintf(fgo2, "{\n")
			fmt.Fprintf(fgo2, "\tsyscall.SetErrno(0)\n")
			fmt.Fprintf(fgo2, "\tr := %s(%s)\n", cname, strings.Join(paramnames, ", "))
			fmt.Fprintf(fgo2, "\te := syscall.GetErrno()\n")
			fmt.Fprintf(fgo2, "\tif e != 0 {\n")
			fmt.Fprintf(fgo2, "\t\treturn r, e\n")
			fmt.Fprintf(fgo2, "\t}\n")
			fmt.Fprintf(fgo2, "\treturn r, nil\n")
			fmt.Fprintf(fgo2, "}\n")
			// declare the C function.
			fmt.Fprintf(fgo2, "//extern %s\n", n.C)
			d.Name = ast.NewIdent(cname)
			l := d.Type.Results.List
			d.Type.Results.List = l[:len(l)-1]
			conf.Fprint(fgo2, fset, d)
			fmt.Fprint(fgo2, "\n")
		}
		return
	}
	conf.Fprint(fgo2, fset, d)
	fmt.Fprint(fgo2, "\n")

	if name == "CString" || name == "GoString" || name == "GoStringN" || name == "GoBytes" {
		// The builtins are already defined in the C prolog.
		return
	}

	var argSize int64
	_, argSize = p.structType(n)

	// C wrapper calls into gcc, passing a pointer to the argument frame.
	fmt.Fprintf(fc, "void _cgo%s%s(void*);\n", cPrefix, n.Mangle)
	fmt.Fprintf(fc, "\n")
	fmt.Fprintf(fc, "void\n")
	if argSize == 0 {
		argSize++
	}
	fmt.Fprintf(fc, "·%s(struct{uint8 x[%d];}p)\n", n.Mangle, argSize)
	fmt.Fprintf(fc, "{\n")
	fmt.Fprintf(fc, "\truntime·cgocall(_cgo%s%s, &p);\n", cPrefix, n.Mangle)
	if n.AddError {
		// gcc leaves errno in first word of interface at end of p.
		// check whether it is zero; if so, turn interface into nil.
		// if not, turn interface into errno.
		// Go init function initializes ·_Cerrno with an os.Errno
		// for us to copy.
		fmt.Fprintln(fc, `	{
			int32 e;
			void **v;
			v = (void**)(&p+1) - 2;	/* v = final two void* of p */
			e = *(int32*)v;
			v[0] = (void*)0xdeadbeef;
			v[1] = (void*)0xdeadbeef;
			if(e == 0) {
				/* nil interface */
				v[0] = 0;
				v[1] = 0;
			} else {
				·_Cerrno(v, e);	/* fill in v as error for errno e */
			}
		}`)
	}
	fmt.Fprintf(fc, "}\n")
	fmt.Fprintf(fc, "\n")
}

// writeOutput creates stubs for a specific source file to be compiled by 6g
// (The comments here say 6g and 6c but the code applies to the 8 and 5 tools too.)
func (p *Package) writeOutput(f *File, srcfile string) {
	base := srcfile
	if strings.HasSuffix(base, ".go") {
		base = base[0 : len(base)-3]
	}
	base = strings.Map(slashToUnderscore, base)
	fgo1 := creat(*objDir + base + ".cgo1.go")
	fgcc := creat(*objDir + base + ".cgo2.c")

	p.GoFiles = append(p.GoFiles, base+".cgo1.go")
	p.GccFiles = append(p.GccFiles, base+".cgo2.c")

	// Write Go output: Go input with rewrites of C.xxx to _C_xxx.
	fmt.Fprintf(fgo1, "// Created by cgo - DO NOT EDIT\n\n")
	conf.Fprint(fgo1, fset, f.AST)

	// While we process the vars and funcs, also write 6c and gcc output.
	// Gcc output starts with the preamble.
	fmt.Fprintf(fgcc, "%s\n", f.Preamble)
	fmt.Fprintf(fgcc, "%s\n", gccProlog)

	for _, n := range f.Name {
		if n.FuncType != nil {
			p.writeOutputFunc(fgcc, n)
		}
	}

	fgo1.Close()
	fgcc.Close()
}

func (p *Package) writeOutputFunc(fgcc *os.File, n *Name) {
	name := n.Mangle
	if name == "_Cfunc_CString" || name == "_Cfunc_GoString" || name == "_Cfunc_GoStringN" || name == "_Cfunc_GoBytes" || p.Written[name] {
		// The builtins are already defined in the C prolog, and we don't
		// want to duplicate function definitions we've already done.
		return
	}
	p.Written[name] = true

	if *gccgo {
		// we don't use wrappers with gccgo.
		return
	}

	ctype, _ := p.structType(n)

	// Gcc wrapper unpacks the C argument struct
	// and calls the actual C function.
	fmt.Fprintf(fgcc, "void\n")
	fmt.Fprintf(fgcc, "_cgo%s%s(void *v)\n", cPrefix, n.Mangle)
	fmt.Fprintf(fgcc, "{\n")
	if n.AddError {
		fmt.Fprintf(fgcc, "\tint e;\n") // assuming 32 bit (see comment above structType)
		fmt.Fprintf(fgcc, "\terrno = 0;\n")
	}
	// We're trying to write a gcc struct that matches 6c/8c/5c's layout.
	// Use packed attribute to force no padding in this struct in case
	// gcc has different packing requirements.  For example,
	// on 386 Windows, gcc wants to 8-align int64s, but 8c does not.
	fmt.Fprintf(fgcc, "\t%s __attribute__((__packed__)) *a = v;\n", ctype)
	fmt.Fprintf(fgcc, "\t")
	if t := n.FuncType.Result; t != nil {
		fmt.Fprintf(fgcc, "a->r = ")
		if c := t.C.String(); c[len(c)-1] == '*' {
			fmt.Fprintf(fgcc, "(const %s) ", t.C)
		}
	}
	fmt.Fprintf(fgcc, "%s(", n.C)
	for i, t := range n.FuncType.Params {
		if i > 0 {
			fmt.Fprintf(fgcc, ", ")
		}
		// We know the type params are correct, because
		// the Go equivalents had good type params.
		// However, our version of the type omits the magic
		// words const and volatile, which can provoke
		// C compiler warnings.  Silence them by casting
		// all pointers to void*.  (Eventually that will produce
		// other warnings.)
		if c := t.C.String(); c[len(c)-1] == '*' {
			fmt.Fprintf(fgcc, "(void*)")
		}
		fmt.Fprintf(fgcc, "a->p%d", i)
	}
	fmt.Fprintf(fgcc, ");\n")
	if n.AddError {
		fmt.Fprintf(fgcc, "\t*(int*)(a->e) = errno;\n")
	}
	fmt.Fprintf(fgcc, "}\n")
	fmt.Fprintf(fgcc, "\n")
}

// Write out the various stubs we need to support functions exported
// from Go so that they are callable from C.
func (p *Package) writeExports(fgo2, fc, fm *os.File) {
	fgcc := creat(*objDir + "_cgo_export.c")
	fgcch := creat(*objDir + "_cgo_export.h")

	fmt.Fprintf(fgcch, "/* Created by cgo - DO NOT EDIT. */\n")
	fmt.Fprintf(fgcch, "%s\n", p.Preamble)
	fmt.Fprintf(fgcch, "%s\n", gccExportHeaderProlog)

	fmt.Fprintf(fgcc, "/* Created by cgo - DO NOT EDIT. */\n")
	fmt.Fprintf(fgcc, "#include \"_cgo_export.h\"\n")

	for _, exp := range p.ExpFunc {
		fn := exp.Func

		// Construct a gcc struct matching the 6c argument and
		// result frame.  The gcc struct will be compiled with
		// __attribute__((packed)) so all padding must be accounted
		// for explicitly.
		ctype := "struct {\n"
		off := int64(0)
		npad := 0
		if fn.Recv != nil {
			t := p.cgoType(fn.Recv.List[0].Type)
			ctype += fmt.Sprintf("\t\t%s recv;\n", t.C)
			off += t.Size
		}
		fntype := fn.Type
		forFieldList(fntype.Params,
			func(i int, atype ast.Expr) {
				t := p.cgoType(atype)
				if off%t.Align != 0 {
					pad := t.Align - off%t.Align
					ctype += fmt.Sprintf("\t\tchar __pad%d[%d];\n", npad, pad)
					off += pad
					npad++
				}
				ctype += fmt.Sprintf("\t\t%s p%d;\n", t.C, i)
				off += t.Size
			})
		if off%p.PtrSize != 0 {
			pad := p.PtrSize - off%p.PtrSize
			ctype += fmt.Sprintf("\t\tchar __pad%d[%d];\n", npad, pad)
			off += pad
			npad++
		}
		forFieldList(fntype.Results,
			func(i int, atype ast.Expr) {
				t := p.cgoType(atype)
				if off%t.Align != 0 {
					pad := t.Align - off%t.Align
					ctype += fmt.Sprintf("\t\tchar __pad%d[%d];\n", npad, pad)
					off += pad
					npad++
				}
				ctype += fmt.Sprintf("\t\t%s r%d;\n", t.C, i)
				off += t.Size
			})
		if off%p.PtrSize != 0 {
			pad := p.PtrSize - off%p.PtrSize
			ctype += fmt.Sprintf("\t\tchar __pad%d[%d];\n", npad, pad)
			off += pad
			npad++
		}
		if ctype == "struct {\n" {
			ctype += "\t\tchar unused;\n" // avoid empty struct
		}
		ctype += "\t}"

		// Get the return type of the wrapper function
		// compiled by gcc.
		gccResult := ""
		if fntype.Results == nil || len(fntype.Results.List) == 0 {
			gccResult = "void"
		} else if len(fntype.Results.List) == 1 && len(fntype.Results.List[0].Names) <= 1 {
			gccResult = p.cgoType(fntype.Results.List[0].Type).C.String()
		} else {
			fmt.Fprintf(fgcch, "\n/* Return type for %s */\n", exp.ExpName)
			fmt.Fprintf(fgcch, "struct %s_return {\n", exp.ExpName)
			forFieldList(fntype.Results,
				func(i int, atype ast.Expr) {
					fmt.Fprintf(fgcch, "\t%s r%d;\n", p.cgoType(atype).C, i)
				})
			fmt.Fprintf(fgcch, "};\n")
			gccResult = "struct " + exp.ExpName + "_return"
		}

		// Build the wrapper function compiled by gcc.
		s := fmt.Sprintf("%s %s(", gccResult, exp.ExpName)
		if fn.Recv != nil {
			s += p.cgoType(fn.Recv.List[0].Type).C.String()
			s += " recv"
		}
		forFieldList(fntype.Params,
			func(i int, atype ast.Expr) {
				if i > 0 || fn.Recv != nil {
					s += ", "
				}
				s += fmt.Sprintf("%s p%d", p.cgoType(atype).C, i)
			})
		s += ")"
		fmt.Fprintf(fgcch, "\nextern %s;\n", s)

		fmt.Fprintf(fgcc, "extern _cgoexp%s_%s(void *, int);\n", cPrefix, exp.ExpName)
		fmt.Fprintf(fgcc, "\n%s\n", s)
		fmt.Fprintf(fgcc, "{\n")
		fmt.Fprintf(fgcc, "\t%s __attribute__((packed)) a;\n", ctype)
		if gccResult != "void" && (len(fntype.Results.List) > 1 || len(fntype.Results.List[0].Names) > 1) {
			fmt.Fprintf(fgcc, "\t%s r;\n", gccResult)
		}
		if fn.Recv != nil {
			fmt.Fprintf(fgcc, "\ta.recv = recv;\n")
		}
		forFieldList(fntype.Params,
			func(i int, atype ast.Expr) {
				fmt.Fprintf(fgcc, "\ta.p%d = p%d;\n", i, i)
			})
		fmt.Fprintf(fgcc, "\tcrosscall2(_cgoexp%s_%s, &a, %d);\n", cPrefix, exp.ExpName, off)
		if gccResult != "void" {
			if len(fntype.Results.List) == 1 && len(fntype.Results.List[0].Names) <= 1 {
				fmt.Fprintf(fgcc, "\treturn a.r0;\n")
			} else {
				forFieldList(fntype.Results,
					func(i int, atype ast.Expr) {
						fmt.Fprintf(fgcc, "\tr.r%d = a.r%d;\n", i, i)
					})
				fmt.Fprintf(fgcc, "\treturn r;\n")
			}
		}
		fmt.Fprintf(fgcc, "}\n")

		// Build the wrapper function compiled by 6c/8c
		goname := exp.Func.Name.Name
		if fn.Recv != nil {
			goname = "_cgoexpwrap" + cPrefix + "_" + fn.Recv.List[0].Names[0].Name + "_" + goname
		}
		fmt.Fprintf(fc, "#pragma dynexport %s %s\n", goname, goname)
		fmt.Fprintf(fc, "extern void ·%s();\n\n", goname)
		fmt.Fprintf(fc, "#pragma textflag 7\n") // no split stack, so no use of m or g
		fmt.Fprintf(fc, "void\n")
		fmt.Fprintf(fc, "_cgoexp%s_%s(void *a, int32 n)\n", cPrefix, exp.ExpName)
		fmt.Fprintf(fc, "{\n")
		fmt.Fprintf(fc, "\truntime·cgocallback(·%s, a, n);\n", goname)
		fmt.Fprintf(fc, "}\n")

		fmt.Fprintf(fm, "int _cgoexp%s_%s;\n", cPrefix, exp.ExpName)

		// Calling a function with a receiver from C requires
		// a Go wrapper function.
		if fn.Recv != nil {
			fmt.Fprintf(fgo2, "func %s(recv ", goname)
			conf.Fprint(fgo2, fset, fn.Recv.List[0].Type)
			forFieldList(fntype.Params,
				func(i int, atype ast.Expr) {
					fmt.Fprintf(fgo2, ", p%d ", i)
					conf.Fprint(fgo2, fset, atype)
				})
			fmt.Fprintf(fgo2, ")")
			if gccResult != "void" {
				fmt.Fprint(fgo2, " (")
				forFieldList(fntype.Results,
					func(i int, atype ast.Expr) {
						if i > 0 {
							fmt.Fprint(fgo2, ", ")
						}
						conf.Fprint(fgo2, fset, atype)
					})
				fmt.Fprint(fgo2, ")")
			}
			fmt.Fprint(fgo2, " {\n")
			fmt.Fprint(fgo2, "\t")
			if gccResult != "void" {
				fmt.Fprint(fgo2, "return ")
			}
			fmt.Fprintf(fgo2, "recv.%s(", exp.Func.Name)
			forFieldList(fntype.Params,
				func(i int, atype ast.Expr) {
					if i > 0 {
						fmt.Fprint(fgo2, ", ")
					}
					fmt.Fprintf(fgo2, "p%d", i)
				})
			fmt.Fprint(fgo2, ")\n")
			fmt.Fprint(fgo2, "}\n")
		}
	}
}

// Write out the C header allowing C code to call exported gccgo functions.
func (p *Package) writeGccgoExports(fgo2, fc, fm *os.File) {
	fgcc := creat(*objDir + "_cgo_export.c")
	fgcch := creat(*objDir + "_cgo_export.h")
	_ = fgcc

	fmt.Fprintf(fgcch, "/* Created by cgo - DO NOT EDIT. */\n")
	fmt.Fprintf(fgcch, "%s\n", p.Preamble)
	fmt.Fprintf(fgcch, "%s\n", gccExportHeaderProlog)
	fmt.Fprintf(fm, "#include \"_cgo_export.h\"\n")

	clean := func(r rune) rune {
		switch {
		case 'A' <= r && r <= 'Z', 'a' <= r && r <= 'z',
			'0' <= r && r <= '9':
			return r
		}
		return '_'
	}
	gccgoSymbolPrefix := strings.Map(clean, *gccgoprefix)

	for _, exp := range p.ExpFunc {
		// TODO: support functions with receivers.
		fn := exp.Func
		fntype := fn.Type

		if !ast.IsExported(fn.Name.Name) {
			fatalf("cannot export unexported function %s with gccgo", fn.Name)
		}

		cdeclBuf := new(bytes.Buffer)
		resultCount := 0
		forFieldList(fntype.Results,
			func(i int, atype ast.Expr) { resultCount++ })
		switch resultCount {
		case 0:
			fmt.Fprintf(cdeclBuf, "void")
		case 1:
			forFieldList(fntype.Results,
				func(i int, atype ast.Expr) {
					t := p.cgoType(atype)
					fmt.Fprintf(cdeclBuf, "%s", t.C)
				})
		default:
			// Declare a result struct.
			fmt.Fprintf(fgcch, "struct %s_result {\n", exp.ExpName)
			forFieldList(fntype.Results,
				func(i int, atype ast.Expr) {
					t := p.cgoType(atype)
					fmt.Fprintf(fgcch, "\t%s r%d;\n", t.C, i)
				})
			fmt.Fprintf(fgcch, "};\n")
			fmt.Fprintf(cdeclBuf, "struct %s_result", exp.ExpName)
		}

		// The function name.
		fmt.Fprintf(cdeclBuf, " "+exp.ExpName)
		gccgoSymbol := fmt.Sprintf("%s.%s.%s", gccgoSymbolPrefix, p.PackageName, exp.Func.Name)
		fmt.Fprintf(cdeclBuf, " (")
		// Function parameters.
		forFieldList(fntype.Params,
			func(i int, atype ast.Expr) {
				if i > 0 {
					fmt.Fprintf(cdeclBuf, ", ")
				}
				t := p.cgoType(atype)
				fmt.Fprintf(cdeclBuf, "%s p%d", t.C, i)
			})
		fmt.Fprintf(cdeclBuf, ")")
		cdecl := cdeclBuf.String()

		fmt.Fprintf(fgcch, "extern %s __asm__(\"%s\");\n", cdecl, gccgoSymbol)
		// Dummy declaration for _cgo_main.c
		fmt.Fprintf(fm, "%s {}\n", cdecl)
	}
}

// Call a function for each entry in an ast.FieldList, passing the
// index into the list and the type.
func forFieldList(fl *ast.FieldList, fn func(int, ast.Expr)) {
	if fl == nil {
		return
	}
	i := 0
	for _, r := range fl.List {
		if r.Names == nil {
			fn(i, r.Type)
			i++
		} else {
			for _ = range r.Names {
				fn(i, r.Type)
				i++
			}
		}
	}
}

func c(repr string, args ...interface{}) *TypeRepr {
	return &TypeRepr{repr, args}
}

// Map predeclared Go types to Type.
var goTypes = map[string]*Type{
	"bool":       {Size: 1, Align: 1, C: c("uchar")},
	"byte":       {Size: 1, Align: 1, C: c("uchar")},
	"int":        {Size: 4, Align: 4, C: c("int")},
	"uint":       {Size: 4, Align: 4, C: c("uint")},
	"rune":       {Size: 4, Align: 4, C: c("int")},
	"int8":       {Size: 1, Align: 1, C: c("schar")},
	"uint8":      {Size: 1, Align: 1, C: c("uchar")},
	"int16":      {Size: 2, Align: 2, C: c("short")},
	"uint16":     {Size: 2, Align: 2, C: c("ushort")},
	"int32":      {Size: 4, Align: 4, C: c("int")},
	"uint32":     {Size: 4, Align: 4, C: c("uint")},
	"int64":      {Size: 8, Align: 8, C: c("int64")},
	"uint64":     {Size: 8, Align: 8, C: c("uint64")},
	"float":      {Size: 4, Align: 4, C: c("float")},
	"float32":    {Size: 4, Align: 4, C: c("float")},
	"float64":    {Size: 8, Align: 8, C: c("double")},
	"complex":    {Size: 8, Align: 8, C: c("__complex float")},
	"complex64":  {Size: 8, Align: 8, C: c("__complex float")},
	"complex128": {Size: 16, Align: 16, C: c("__complex double")},
}

// Map an ast type to a Type.
func (p *Package) cgoType(e ast.Expr) *Type {
	switch t := e.(type) {
	case *ast.StarExpr:
		x := p.cgoType(t.X)
		return &Type{Size: p.PtrSize, Align: p.PtrSize, C: c("%s*", x.C)}
	case *ast.ArrayType:
		if t.Len == nil {
			return &Type{Size: p.PtrSize + 8, Align: p.PtrSize, C: c("GoSlice")}
		}
	case *ast.StructType:
		// TODO
	case *ast.FuncType:
		return &Type{Size: p.PtrSize, Align: p.PtrSize, C: c("void*")}
	case *ast.InterfaceType:
		return &Type{Size: 2 * p.PtrSize, Align: p.PtrSize, C: c("GoInterface")}
	case *ast.MapType:
		return &Type{Size: p.PtrSize, Align: p.PtrSize, C: c("GoMap")}
	case *ast.ChanType:
		return &Type{Size: p.PtrSize, Align: p.PtrSize, C: c("GoChan")}
	case *ast.Ident:
		// Look up the type in the top level declarations.
		// TODO: Handle types defined within a function.
		for _, d := range p.Decl {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Name.Name == t.Name {
					return p.cgoType(ts.Type)
				}
			}
		}
		if def := typedef[t.Name]; def != nil {
			return def
		}
		if t.Name == "uintptr" {
			return &Type{Size: p.PtrSize, Align: p.PtrSize, C: c("uintptr")}
		}
		if t.Name == "string" {
			return &Type{Size: p.PtrSize + 4, Align: p.PtrSize, C: c("GoString")}
		}
		if t.Name == "error" {
			return &Type{Size: 2 * p.PtrSize, Align: p.PtrSize, C: c("GoInterface")}
		}
		if r, ok := goTypes[t.Name]; ok {
			if r.Align > p.PtrSize {
				r.Align = p.PtrSize
			}
			return r
		}
		error_(e.Pos(), "unrecognized Go type %s", t.Name)
		return &Type{Size: 4, Align: 4, C: c("int")}
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if ok && id.Name == "unsafe" && t.Sel.Name == "Pointer" {
			return &Type{Size: p.PtrSize, Align: p.PtrSize, C: c("void*")}
		}
	}
	error_(e.Pos(), "Go type not supported in export: %s", gofmt(e))
	return &Type{Size: 4, Align: 4, C: c("int")}
}

const gccProlog = `
// Usual nonsense: if x and y are not equal, the type will be invalid
// (have a negative array count) and an inscrutable error will come
// out of the compiler and hopefully mention "name".
#define __cgo_compile_assert_eq(x, y, name) typedef char name[(x-y)*(x-y)*-2+1];

// Check at compile time that the sizes we use match our expectations.
#define __cgo_size_assert(t, n) __cgo_compile_assert_eq(sizeof(t), n, _cgo_sizeof_##t##_is_not_##n)

__cgo_size_assert(char, 1)
__cgo_size_assert(short, 2)
__cgo_size_assert(int, 4)
typedef long long __cgo_long_long;
__cgo_size_assert(__cgo_long_long, 8)
__cgo_size_assert(float, 4)
__cgo_size_assert(double, 8)

#include <errno.h>
#include <string.h>
`

const builtinProlog = `
typedef struct { char *p; int n; } _GoString_;
typedef struct { char *p; int n; int c; } _GoBytes_;
_GoString_ GoString(char *p);
_GoString_ GoStringN(char *p, int l);
_GoBytes_ GoBytes(void *p, int n);
char *CString(_GoString_);
`

const cProlog = `
#include "runtime.h"
#include "cgocall.h"

void ·_Cerrno(void*, int32);

void
·_Cfunc_GoString(int8 *p, String s)
{
	s = runtime·gostring((byte*)p);
	FLUSH(&s);
}

void
·_Cfunc_GoStringN(int8 *p, int32 l, String s)
{
	s = runtime·gostringn((byte*)p, l);
	FLUSH(&s);
}

void
·_Cfunc_GoBytes(int8 *p, int32 l, Slice s)
{
	s = runtime·gobytes((byte*)p, l);
	FLUSH(&s);
}

void
·_Cfunc_CString(String s, int8 *p)
{
	p = runtime·cmalloc(s.len+1);
	runtime·memmove((byte*)p, s.str, s.len);
	p[s.len] = 0;
	FLUSH(&p);
}
`

const cPrologGccgo = `
#include <stdint.h>
#include <string.h>

struct __go_string {
	const unsigned char *__data;
	int __length;
};

typedef struct __go_open_array {
	void* __values;
	int __count;
	int __capacity;
} Slice;

struct __go_string __go_byte_array_to_string(const void* p, int len);
struct __go_open_array __go_string_to_byte_array (struct __go_string str);

const char *CString(struct __go_string s) {
	return strndup((const char*)s.__data, s.__length);
}

struct __go_string GoString(char *p) {
	int len = (p != NULL) ? strlen(p) : 0;
	return __go_byte_array_to_string(p, len);
}

struct __go_string GoStringN(char *p, int n) {
	return __go_byte_array_to_string(p, n);
}

Slice GoBytes(char *p, int n) {
	struct __go_string s = { (const unsigned char *)p, n };
	return __go_string_to_byte_array(s);
}
`

const gccExportHeaderProlog = `
typedef unsigned int uint;
typedef signed char schar;
typedef unsigned char uchar;
typedef unsigned short ushort;
typedef long long int64;
typedef unsigned long long uint64;
typedef __SIZE_TYPE__ uintptr;

typedef struct { char *p; int n; } GoString;
typedef void *GoMap;
typedef void *GoChan;
typedef struct { void *t; void *v; } GoInterface;
`
