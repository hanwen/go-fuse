// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"testing"
)

func TestBufferPool(t *testing.T) {
	bp := bufferPool{}
	size := 1500
	buf1 := bp.AllocBuffer(uint32(size))
	if len(buf1) != size {
		t.Errorf("Expected buffer of %d bytes, got %d bytes", size, len(buf1))
	}
	bp.FreeBuffer(buf1)

	// tried testing to see if we get buf1 back if we ask again,
	// but it's not guaranteed and sometimes fails
}
