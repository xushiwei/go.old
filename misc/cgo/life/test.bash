#!/bin/sh
# Copyright 2010 The Go Authors.  All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e
gomake life
echo '*' life >run.out
./life >>run.out
diff run.out golden.out
gomake clean
