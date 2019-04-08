// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// A Go mirror of libfuse's hello.c

package main

import (
	"context"
	"flag"
	"log"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/nodefs"
)

type HelloRoot struct {
	nodefs.Inode
}

func (r *HelloRoot) OnAdd(ctx context.Context) {
	ch := r.NewPersistentInode(
		ctx, &nodefs.MemRegularFile{
			Data: []byte("file.txt"),
			Attr: fuse.Attr{
				Mode: 0644,
			},
		}, nodefs.StableAttr{Ino: 2})
	r.AddChild("file.txt", ch, false)
}

func (r *HelloRoot) Getattr(ctx context.Context, fh nodefs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

var _ = (nodefs.Getattrer)((*HelloRoot)(nil))
var _ = (nodefs.OnAdder)((*HelloRoot)(nil))

func main() {
	debug := flag.Bool("debug", false, "print debug data")
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	opts := &nodefs.Options{}
	opts.Debug = *debug
	server, err := nodefs.Mount(flag.Arg(0), &HelloRoot{}, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	server.Wait()
}
