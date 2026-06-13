// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// numPossibleCPUs returns the kernel's num_possible_cpus(). The FUSE
// kernel module sizes its ring queue array to this, and refuses to
// allocate any FUSE request until every qid in [0, num_possible_cpus)
// has been registered (see fuse_block_alloc + is_ring_ready in
// fs/fuse/dev_uring.c). We must register exactly that many queues.
//
// Reads /sys/devices/system/cpu/possible, matching what
// num_possible_cpus() reflects from userspace.
func numPossibleCPUs() (int, error) {
	b, err := os.ReadFile("/sys/devices/system/cpu/possible")
	if err != nil {
		return 0, err
	}
	// Format is "0-N" or "0-3,5-7" etc. Sum the ranges.
	s := strings.TrimSpace(string(b))
	total := 0
	for _, part := range strings.Split(s, ",") {
		lo, hi, ok := strings.Cut(part, "-")
		if !ok {
			hi = lo
		}
		l, err := strconv.Atoi(lo)
		if err != nil {
			return 0, err
		}
		h, err := strconv.Atoi(hi)
		if err != nil {
			return 0, err
		}
		total += h - l + 1
	}
	return total, nil
}

// uringEnabled reports whether the io_uring transport was negotiated at
// INIT time: the user asked for it and the kernel offered the capability.
func (ms *Server) uringEnabled() bool {
	return ms.opts.EnableIoUring &&
		ms.kernelSettings.Flags64()&CAP_OVER_IO_URING != 0
}

// startUring spawns the FUSE-over-io_uring transport. Must be called
// after handleInit.
//
// The kernel sizes its queue array to num_possible_cpus() and blocks all
// new request allocations until every qid in that range has at least
// one registered entry (see fuse_block_alloc + is_ring_ready in
// fs/fuse/dev_uring.c). So we must spawn exactly num_possible_cpus()
// queues, with a small number of entries each.
func (ms *Server) startUring() error {
	nq, err := numPossibleCPUs()
	if err != nil {
		return fmt.Errorf("num_possible_cpus: %w", err)
	}

	payloadSize := ms.opts.MaxWrite
	if payloadSize < 4096 {
		payloadSize = 4096
	}

	for qid := 0; qid < nq; qid++ {
		q, err := newUringQueue(&ms.protocolServer, ms.mountFd, uint16(qid),
			uringDefaultEntries, payloadSize)
		if err != nil {
			return fmt.Errorf("uring queue %d: %w", qid, err)
		}
		if ms.opts.Debug {
			q.debugf = ms.opts.Logger.Printf
		}
		ms.uringQueues = append(ms.uringQueues, q)

		ms.loops.Add(1)
		go func() {
			defer ms.loops.Done()
			// Pin to the qid's CPU. The kernel routes requests by
			// task_cpu(current) and defers CQE delivery onto the
			// submitting task via io_uring_cmd_complete_in_task.
			// Without pinning, a goroutine that migrates between
			// submit and the deferred completion runs that
			// completion on a different CPU than the kernel
			// expected — racy under load.
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var set unix.CPUSet
			set.Set(int(q.qid))
			if err := unix.SchedSetaffinity(0, &set); err != nil {
				ms.opts.Logger.Printf("uring queue %d: sched_setaffinity: %v", q.qid, err)
			}
			if err := q.Run(); err != nil {
				ms.opts.Logger.Printf("uring queue %d exit: %v", q.qid, err)
			}
		}()
	}
	return nil
}

// stopUring tears down all uring queues. Safe to call multiple times.
func (ms *Server) stopUring() {
	for _, q := range ms.uringQueues {
		q.Close()
	}
	ms.uringQueues = nil
}
