#!/bin/sh
# Copyright 2009 The Go Authors.  All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e
go build hello.go
go build fib.go
go build chain.go

echo '*' hello >run.out
./hello >>run.out
echo '*' fib >>run.out
./fib >>run.out
echo '*' chain >>run.out
./chain >>run.out
diff run.out golden.out

rm -f hello fib chain

