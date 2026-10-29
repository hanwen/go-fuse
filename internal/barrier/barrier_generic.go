// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !amd64 && !arm64

package barrier

import "sync/atomic"

// sync/atomic operations are sequentially consistent, so any of them
// acts as a full barrier.
var fence uint32

func Write() { atomic.AddUint32(&fence, 1) }
func Read()  { atomic.AddUint32(&fence, 1) }
func Full()  { atomic.AddUint32(&fence, 1) }

//go:noinline
func LoadUint16(ptr *uint16) uint16 {
	return *ptr
}
