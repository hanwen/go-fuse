// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"log"
	"net"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

const _HUGETLBFS_MAGIC = 0x958458f6

func getFDHugepagesize(fd int) int {
	var fs syscall.Statfs_t
	var err error
	for {
		err = syscall.Fstatfs(fd, &fs)
		if err != syscall.EINTR {
			break
		}
	}

	if err == nil && fs.Type == _HUGETLBFS_MAGIC {
		return int(fs.Bsize)
	}
	return 0
}

func composeMask(fs []int) uint64 {
	var mask uint64
	for _, f := range fs {
		mask |= (uint64(0x1) << f)
	}
	return mask
}

func ServeFS(sockpath string, rawFS fuse.RawFileSystem, opts *fuse.MountOptions) {
	l, err := net.ListenUnix("unix", &net.UnixAddr{sockpath, "unix"})
	if err != nil {
		log.Fatal("Listen", err)
	}

	ps := fuse.NewProtocolServer(rawFS, opts)
	opts.DisableSplice = true
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			break
		}

		dev := NewDevice(func(vqe *VirtqElem) int {
			n, _ := ps.HandleRequest(vqe.Read, vqe.Write)
			return n
		})
		// dev.Debug = true
		srv := NewServer(conn, dev)
		srv.Debug = true
		if err := srv.Serve(); err != nil {
			log.Printf("Serve: %v %T", err, err)
		}
	}
}
