// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/fuse"
)

type loopbackDirStream struct {
	buf  []byte
	todo []byte
	fd   int
}

func NewLoopbackDirStream(name string) (DirStream, fuse.Status) {
	fd, err := syscall.Open(name, syscall.O_DIRECTORY, 0755)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	ds := &loopbackDirStream{
		buf: make([]byte, 4096),
		fd:  fd,
	}

	if err := ds.load(); !err.Ok() {
		ds.Close()
		return nil, err
	}
	return ds, fuse.OK
}

func (ds *loopbackDirStream) Close() {
	syscall.Close(ds.fd)
}

func (ds *loopbackDirStream) HasNext() bool {
	return len(ds.todo) > 0
}

func (ds *loopbackDirStream) Next() (fuse.DirEntry, fuse.Status) {
	de := (*syscall.Dirent)(unsafe.Pointer(&ds.todo[0]))

	nameBytes := ds.todo[unsafe.Offsetof(syscall.Dirent{}.Name):de.Reclen]
	ds.todo = ds.todo[de.Reclen:]

	for l := len(nameBytes); l > 0; l-- {
		if nameBytes[l-1] != 0 {
			break
		}
		nameBytes = nameBytes[:l-1]
	}
	result := fuse.DirEntry{
		Ino:  de.Ino,
		Mode: (uint32(de.Type) << 12),
		Name: string(nameBytes),
	}
	return result, ds.load()
}

func (ds *loopbackDirStream) load() fuse.Status {
	if len(ds.todo) > 0 {
		return fuse.OK
	}

	n, err := syscall.Getdents(ds.fd, ds.buf)
	if err != nil {
		return fuse.ToStatus(err)
	}
	ds.todo = ds.buf[:n]
	return fuse.OK
}
