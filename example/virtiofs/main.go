// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"log"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/vhostuser"
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	flag.Parse()

	sockpath := flag.Arg(0)
	orig := flag.Arg(1)
	opts := &fs.Options{}

	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		log.Fatal("Listen", err)
	}

	opts.Debug = true
	opts.Logger = log.Default()
	opts.MountOptions.Logger = opts.Logger
	rawFS := fs.NewNodeFS(root, opts)

	vhostuser.ServeFS(sockpath, rawFS, &opts.MountOptions)
}
