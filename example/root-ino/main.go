// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program demonstrates setting the inode number on
// a root directory at mount time.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type HelloRoot struct {
	fs.Inode
}

func (r *HelloRoot) OnAdd(ctx context.Context) {
	ch := r.NewPersistentInode(
		ctx, &fs.MemRegularFile{
			Data: []byte("file.txt"),
			Attr: fuse.Attr{
				Mode: 0644,
			},
		}, fs.StableAttr{Ino: 2})
	r.AddChild("file.txt", ch, false)
}

func (r *HelloRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

var _ = (fs.NodeGetattrer)((*HelloRoot)(nil))
var _ = (fs.NodeOnAdder)((*HelloRoot)(nil))

func main() {
	debug := flag.Bool("debug", false, "print debug data")
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	opts := &fs.Options{}
	opts.Debug = *debug

	// set root Ino
	var ino uint64 = 42
	opts.RootStableAttr = &fs.StableAttr{Ino: ino}

	server, err := fs.Mount(flag.Arg(0), &HelloRoot{}, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	// check root Ino
	fileinfo, err := os.Stat(flag.Arg(0))
	if err != nil {
		log.Fatalf("Stat fail: %v\n", err)
	}
	sys := fileinfo.Sys()
	stat, ok := sys.(*syscall.Stat_t)
	if !ok {
		log.Fatalf("syscall stat fail\n")
	}
	if stat.Ino != ino {
		log.Fatalf("root inode set fail: expect %v got %v\n", ino, stat.Ino)
	}

	server.Wait()
}
