// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fuseclient sends FUSE requests to a fuse.ProtocolServer
// in-process, acting as a small FUSE kernel without a mount. It is
// used to serve WinFsp, and can drive file systems in tests.
//
// It speaks the Linux variant of the protocol. It is not built on
// macOS, whose FUSE protocol differs.
package fuseclient
