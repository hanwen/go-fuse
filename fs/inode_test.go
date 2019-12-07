// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"strings"
	"syscall"
	"testing"
)

// TestInodePath check that Inode.Path returns ".deleted" if an Inode is
// disconnected from the hierarchy (=orphaned)
func TestInodePath(t *testing.T) {
	// Very simple hierarchy:
	// rootDir -> subDir
	rootDir := &Inode{
		stableAttr: StableAttr{Ino: 1, Mode: syscall.S_IFDIR},
		children:   make(map[string]*Inode),
	}
	subDir := &Inode{
		stableAttr: StableAttr{Ino: 2, Mode: syscall.S_IFDIR},
		parents:    make(map[parentData]struct{}),
	}
	rootDir.children["subDir"] = subDir
	subDir.parents[parentData{"subDir", rootDir}] = struct{}{}

	// sanity check
	p := rootDir.Path(rootDir)
	if p != "" {
		t.Errorf("want %q, got %q", "", p)
	}
	p = subDir.Path(rootDir)
	if p != "subDir" {
		t.Errorf("want %q, got %q", "", p)
	}

	// remove subDir from the hierarchy
	delete(rootDir.children, "subDir")
	delete(subDir.parents, parentData{"subDir", rootDir})
	p = subDir.Path(rootDir)

	if !strings.HasPrefix(p, ".go-fuse") {
		t.Errorf("want %q, got %q", ".go-fuse", p)
	}
}
