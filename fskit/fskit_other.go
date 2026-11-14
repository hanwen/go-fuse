// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin

// Package fskit implements a FUSE file system frontend on macOS using the
// macFUSE 5.3.0+ FSKit user-space framework and fuse.ProtocolServer.
package fskit

import (
	"errors"

	"github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
)

// ErrUnsupported is returned when fskit is called on a non-Darwin OS.
var ErrUnsupported = errors.New("fskit: macOS FSKit backend is only supported on darwin")

// Available reports whether the macFUSE FSKit framework is available.
func Available() bool {
	return false
}

// Server represents an active FSKit file system mount session.
type Server struct{}

// ProtocolServer returns nil on non-Darwin platforms.
func (s *Server) ProtocolServer() *fuse.ProtocolServer { return nil }

// MountPoint returns empty string on non-Darwin platforms.
func (s *Server) MountPoint() string { return "" }

// Wait returns immediately on non-Darwin platforms.
func (s *Server) Wait() {}

// Unmount returns nil on non-Darwin platforms.
func (s *Server) Unmount() error { return nil }

// MountRaw returns ErrUnsupported on non-Darwin platforms.
func MountRaw(mountPoint string, rawFS fuse.RawFileSystem, opts *fuse.MountOptions) (*Server, error) {
	return nil, ErrUnsupported
}

// Mount returns ErrUnsupported on non-Darwin platforms.
func Mount(dir string, root fs.InodeEmbedder, options *fs.Options) (*Server, error) {
	return nil, ErrUnsupported
}
