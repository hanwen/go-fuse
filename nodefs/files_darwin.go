// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

func (f *loopbackFile) Allocate(ctx context.Context, off uint64, sz uint64, mode uint32) fuse.Status {
	return fuse.ENOSYS
}

func (f *loopbackFile) Utimens(ctx context.Context, a *time.Time, m *time.Time) fuse.Status {
	return fuse.ENOSYS
}
