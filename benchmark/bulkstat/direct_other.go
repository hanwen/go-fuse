// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

package main

// oDirect is 0 where O_DIRECT is unavailable; reads and writes then go
// through the page cache unless the server sets FOPEN_DIRECT_IO.
const oDirect = 0
