// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows

package fuse

func (s *Server) fdBacking() *Backing {
	return enosysBacking()
}
