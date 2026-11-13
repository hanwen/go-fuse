// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xattr

import "syscall"

// ENOATTR indicates that an extended attribute was not present.
const ENOATTR = syscall.ENODATA
