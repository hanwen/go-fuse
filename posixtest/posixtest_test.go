// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package posixtest

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var probeDir = flag.String("posixdir", "", "dir to test")

func TestAll(t *testing.T) {
	for k, fn := range All {
		if k == "FcntlFlockLocksFile" {
			// TODO - fix this test.
			continue
		}

		if k == "OpenSymlinkRace" {
			continue
		}
		dir := *probeDir
		if dir == "" {
			dir = t.TempDir()
		}
		t.Run(k, func(t *testing.T) {
			sub := filepath.Join(dir, k)
			if err := os.MkdirAll(sub, 0755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			fn(t, sub)
		})
	}
}
