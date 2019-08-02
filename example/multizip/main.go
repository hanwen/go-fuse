// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is main program driver for MultiZipFs from
// github.com/hanwen/go-fuse/zipfs, a filesystem for mounting multiple
// read-only archives. It can be used by symlinking to an archive file
// from the config/ subdirectory.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/zipfs"
)

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "debug on")
	flag.Parse()
	if flag.NArg() < 1 {
		_, prog := filepath.Split(os.Args[0])
		fmt.Printf("usage: %s MOUNTPOINT\n", prog)
		os.Exit(2)
	}

	root := &zipfs.MultiZipFs{}
	sec := time.Second
	opts := fs.Options{
		EntryTimeout: &sec,
		AttrTimeout:  &sec,
	}
	opts.Debug = *debug
	server, err := fs.Mount(flag.Arg(0), root, &opts)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	server.Serve()
}
