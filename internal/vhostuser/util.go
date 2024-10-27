// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"log"
	"syscall"
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

// readLoop reads kick eventfd notifications and processes virtqueue elements.
//
// Locking follows the virtiofsd model (fuse_virtio.c):
//   - dispatchMu read lock is held only while draining the avail ring
//     (popQueue calls).  This blocks concurrent control-plane messages
//     (ADD_MEM_REG, SET_VRING_*, etc.) which take the write lock.
//   - The lock is released before spawning request goroutines, so
//     control-plane messages can be processed concurrently with in-flight
//     FUSE requests.  That is safe because ADD_MEM_REG only adds regions
//     and never invalidates pointers already held by a request.
func (vq *Virtq) readLoop(handle func(data *VirtqElem) int) {
	defer close(vq.control.done)
	for {
		select {
		case <-vq.control.cancel:
			return
		default:
		}

		var id [8]byte
		_, err := syscall.Read(vq.KickFD, id[:])
		if err != nil {
			log.Printf("read: %v", err)
			return
		}

		// Process the batch without holding any lock.
		for _, data := range vq.popBatch() {
			data := data // TODO: unnecessary starting go1.22
			go func() {
				for _, e := range data.Write {
					clear(e)
				}

				if *vq.Debug {
					for i, e := range data.Read {
						log.Printf("read %d: %q (%d)", i, e, len(e))
					}
					outlens := []int{}
					for _, e := range data.Write {
						outlens = append(outlens, len(e))
					}
					log.Printf("id %d: write space: %v", data.index, outlens)
				}
				var n int
				if handle != nil {
					n = handle(data)
				}

				if *vq.Debug {
					for i, e := range data.Write {
						log.Printf("write %d: %q (%d)", i, e, len(e))
					}
				}
				vq.pushQueue(data, n)
				vq.queueNotify()
			}()
		}
	}
}
