// Copyright 2025 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"testing"
)

// Note: Writting too long and too many files might encounter error 'No space left on device':
// "dd failed: exit status 1: output=dd: error writing '/tmp/BenchmarkGoFuseFDWrite1491954343/006/foo_4.txt': No space left on device"

func BenchmarkGoFuseFDWrite(b *testing.B) {
	mnt := setupLoopbackFs(b, false)
	doBenchmark(mnt, b, "direct", opWrite)
}

func BenchmarkLibfuseHPWrite(b *testing.B) {
	mnt := setupLibfuseFs(b, false)
	doBenchmark(mnt, b, "", opWrite)
}
