// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"github.com/hanwen/go-fuse/fuse"
)

type dirArray struct {
	entries []fuse.DirEntry
}

func (a *dirArray) HasNext() bool {
	return len(a.entries) > 0
}

func (a *dirArray) Next() (fuse.DirEntry, fuse.Status) {
	e := a.entries[0]
	a.entries = a.entries[1:]
	return e, fuse.OK
}

func (a *dirArray) Close() {

}

func NewListDirStream(list []fuse.DirEntry) DirStream {
	return &dirArray{list}
}
