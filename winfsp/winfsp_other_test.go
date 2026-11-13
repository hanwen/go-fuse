// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows || !(amd64 || arm64)

package winfsp

import (
	"errors"
	"testing"

	"github.com/hanwen/go-fuse/v3/internal/memfs"
)

// TestMountUnsupported checks that platforms without WinFsp support,
// including 32-bit Windows, fail cleanly.
func TestMountUnsupported(t *testing.T) {
	if Available() {
		t.Error("Available() = true")
	}
	if _, err := Mount("", memfs.NewRoot(), nil); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Mount: %v, want ErrUnsupported", err)
	}
}
