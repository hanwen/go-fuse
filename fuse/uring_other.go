// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

package fuse

// uringQueue is a stub on non-Linux platforms. FUSE-over-io_uring is a
// Linux-only kernel feature; Server.uringQueues stays nil here and the
// negotiation in doInit can never set CAP_OVER_IO_URING since the kernel
// will not offer it.
type uringQueue struct{}

func (*uringQueue) Close() error { return nil }

func (ms *Server) uringEnabled() bool { return false }
func (ms *Server) startUring() error  { return nil }
func (ms *Server) stopUring()         {}
