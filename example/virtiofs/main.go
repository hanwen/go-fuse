// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is main program driver for the loopback filesystem from
// github.com/hanwen/go-fuse/fs/, a filesystem that shunts operations
// to an underlying file system.
package main

import (
	"flag"
	"log"
	"net"

	"github.com/hanwen/go-fuse/v2/vhostuser"
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	flag.Parse()

	sockpath := flag.Arg(0)

	l, err := net.ListenUnix("unix", &net.UnixAddr{sockpath, "unix"})
	if err != nil {
		log.Fatal("Listen", err)
	}
	log.Println("listening")
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			break
		}

		dev := vhostuser.NewFSDevice()
		srv := vhostuser.NewServer(conn, dev)
		if err := srv.Serve(); err != nil {
			log.Printf("Serve: %v %T", err, err)
		}
	}
}
