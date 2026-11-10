// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !freebsd

package posixtest

// o_DIRECT is 0 where open(2) has no O_DIRECT.
const o_DIRECT = 0
