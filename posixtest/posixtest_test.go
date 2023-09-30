// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package posixtest

import "testing"

func TestAll(t *testing.T) {
	for k, fn := range All {
		if k == "FcntlFlockLocksFile" {
			// TODO - fix this test.
			continue
		}
		t.Run(k, func(t *testing.T) {
			dir := t.TempDir()
			fn(t, dir)
		})
	}
}
