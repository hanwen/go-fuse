// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows || !(amd64 || arm64)

package winfsp

import (
	"errors"
	"fmt"

	"github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
)

// errUnsupported is returned on platforms other than Windows on amd64
// and arm64.
var errUnsupported = fmt.Errorf("winfsp: only supported on Windows (amd64, arm64): %w", errors.ErrUnsupported)

// Available reports whether WinFsp is installed.
func Available() bool { return false }

// Server is a mounted WinFsp file system.
type Server struct{}

// MountPoint returns the mount point.
func (s *Server) MountPoint() string { return "" }

// Wait waits until the file system is unmounted.
func (s *Server) Wait() {}

// Unmount unmounts the file system.
func (s *Server) Unmount() error { return errUnsupported }

// MountRaw returns an error wrapping errors.ErrUnsupported.
func MountRaw(mountPoint string, raw fuse.RawFileSystem, opts *fuse.MountOptions) (*Server, error) {
	return nil, errUnsupported
}

// Mount returns an error wrapping errors.ErrUnsupported.
func Mount(mountPoint string, root fs.InodeEmbedder, options *fs.Options) (*Server, error) {
	return nil, errUnsupported
}
