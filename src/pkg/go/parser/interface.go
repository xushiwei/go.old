// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains the exported entry points for invoking the parser.

package parser

import (
	"bytes"
	"errors"
	"go/ast"
	"go/scanner"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// If src != nil, readSource converts src to a []byte if possible;
// otherwise it returns an error. If src == nil, readSource returns
// the result of reading the file specified by filename.
//
func readSource(filename string, src interface{}) ([]byte, error) {
	if src != nil {
		switch s := src.(type) {
		case string:
			return []byte(s), nil
		case []byte:
			return s, nil
		case *bytes.Buffer:
			// is io.Reader, but src is already available in []byte form
			if s != nil {
				return s.Bytes(), nil
			}
		case io.Reader:
			var buf bytes.Buffer
			_, err := io.Copy(&buf, s)
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		default:
			return nil, errors.New("invalid source")
		}
	}

	return ioutil.ReadFile(filename)
}

func (p *parser) errors() error {
	mode := scanner.Sorted
	if p.mode&SpuriousErrors == 0 {
		mode = scanner.NoMultiples
	}
	return p.GetError(mode)
}

// ParseExpr parses a Go expression and returns the corresponding
// AST node. The fset, filename, and src arguments have the same interpretation
// as for ParseFile. If there is an error, the result expression
// may be nil or contain a partial AST.
//
func ParseExpr(fset *token.FileSet, filename string, src interface{}) (ast.Expr, error) {
	data, err := readSource(filename, src)
	if err != nil {
		return nil, err
	}

	var p parser
	p.init(fset, filename, data, 0)
	x := p.parseRhs()
	if p.tok == token.SEMICOLON {
		p.next() // consume automatically inserted semicolon, if any
	}
	p.expect(token.EOF)

	return x, p.errors()
}

// ParseStmtList parses a list of Go statements and returns the list
// of corresponding AST nodes. The fset, filename, and src arguments have the same
// interpretation as for ParseFile. If there is an error, the node
// list may be nil or contain partial ASTs.
//
func ParseStmtList(fset *token.FileSet, filename string, src interface{}) ([]ast.Stmt, error) {
	data, err := readSource(filename, src)
	if err != nil {
		return nil, err
	}

	var p parser
	p.init(fset, filename, data, 0)
	list := p.parseStmtList()
	p.expect(token.EOF)

	return list, p.errors()
}

// ParseDeclList parses a list of Go declarations and returns the list
// of corresponding AST nodes. The fset, filename, and src arguments have the same
// interpretation as for ParseFile. If there is an error, the node
// list may be nil or contain partial ASTs.
//
func ParseDeclList(fset *token.FileSet, filename string, src interface{}) ([]ast.Decl, error) {
	data, err := readSource(filename, src)
	if err != nil {
		return nil, err
	}

	var p parser
	p.init(fset, filename, data, 0)
	list := p.parseDeclList()
	p.expect(token.EOF)

	return list, p.errors()
}

// ParseFile parses the source code of a single Go source file and returns
// the corresponding ast.File node. The source code may be provided via
// the filename of the source file, or via the src parameter.
//
// If src != nil, ParseFile parses the source from src and the filename is
// only used when recording position information. The type of the argument
// for the src parameter must be string, []byte, or io.Reader.
//
// If src == nil, ParseFile parses the file specified by filename.
//
// The mode parameter controls the amount of source text parsed and other
// optional parser functionality. Position information is recorded in the
// file set fset.
//
// If the source couldn't be read, the returned AST is nil and the error
// indicates the specific failure. If the source was read but syntax
// errors were found, the result is a partial AST (with ast.BadX nodes
// representing the fragments of erroneous source code). Multiple errors
// are returned via a scanner.ErrorList which is sorted by file position.
//
func ParseFile(fset *token.FileSet, filename string, src interface{}, mode uint) (*ast.File, error) {
	data, err := readSource(filename, src)
	if err != nil {
		return nil, err
	}

	var p parser
	p.init(fset, filename, data, mode)
	file := p.parseFile() // parseFile reads to EOF

	return file, p.errors()
}

// ParseFiles calls ParseFile for each file in the filenames list and returns
// a map of package name -> package AST with all the packages found. The mode
// bits are passed to ParseFile unchanged. Position information is recorded
// in the file set fset.
//
// Files with parse errors are ignored. In this case the map of packages may
// be incomplete (missing packages and/or incomplete packages) and the first
// error encountered is returned.
//
func ParseFiles(fset *token.FileSet, filenames []string, mode uint) (pkgs map[string]*ast.Package, first error) {
	pkgs = make(map[string]*ast.Package)
	for _, filename := range filenames {
		if src, err := ParseFile(fset, filename, nil, mode); err == nil {
			name := src.Name.Name
			pkg, found := pkgs[name]
			if !found {
				// TODO(gri) Use NewPackage here; reconsider ParseFiles API.
				pkg = &ast.Package{name, nil, nil, make(map[string]*ast.File)}
				pkgs[name] = pkg
			}
			pkg.Files[filename] = src
		} else if first == nil {
			first = err
		}
	}
	return
}

// ParseDir calls ParseFile for the files in the directory specified by path and
// returns a map of package name -> package AST with all the packages found. If
// filter != nil, only the files with os.FileInfo entries passing through the filter
// are considered. The mode bits are passed to ParseFile unchanged. Position
// information is recorded in the file set fset.
//
// If the directory couldn't be read, a nil map and the respective error are
// returned. If a parse error occurred, a non-nil but incomplete map and the
// error are returned.
//
func ParseDir(fset *token.FileSet, path string, filter func(os.FileInfo) bool, mode uint) (map[string]*ast.Package, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	list, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	filenames := make([]string, len(list))
	n := 0
	for _, d := range list {
		if filter == nil || filter(d) {
			filenames[n] = filepath.Join(path, d.Name())
			n++
		}
	}
	filenames = filenames[0:n]

	return ParseFiles(fset, filenames, mode)
}
