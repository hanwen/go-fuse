// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package posixtest file systems for generic posix conformance.
package posixtest

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
)

// listFds lists the open file descriptors for process "pid". Pass pid=0 for
// ourselves. Pass a prefix to ignore all paths that do not start with "prefix".
//
// Copied from https://github.com/rfjakob/gocryptfs/blob/master/tests/test_helpers/mount_unmount.go#L191
func listFds(pid int, prefix string) []string {
	// We need /proc to get the list of fds for other processes. Only exists
	// on Linux.
	if runtime.GOOS != "linux" && pid > 0 {
		return nil
	}
	// Both Linux and MacOS have /dev/fd
	dir := "/dev/fd"
	if pid > 0 {
		dir = fmt.Sprintf("/proc/%d/fd", pid)
	}
	f, err := os.Open(dir)
	if err != nil {
		fmt.Printf("ListFds: %v\n", err)
		return nil
	}
	defer f.Close()
	// Note: Readdirnames filters "." and ".."
	names, err := f.Readdirnames(0)
	if err != nil {
		log.Panic(err)
	}
	var out []string
	var filtered []string
	for _, n := range names {
		fdPath := dir + "/" + n
		fi, err := os.Lstat(fdPath)
		if err != nil {
			// fd was closed in the meantime
			continue
		}
		if fi.Mode()&0400 > 0 {
			n += "r"
		}
		if fi.Mode()&0200 > 0 {
			n += "w"
		}
		target, err := os.Readlink(fdPath)
		if err != nil {
			// fd was closed in the meantime
			continue
		}
		if strings.HasPrefix(target, "pipe:") || strings.HasPrefix(target, "anon_inode:[eventpoll]") {
			// The Go runtime creates pipes on demand for splice(), which
			// creates spurious test failures. Ignore all pipes.
			// Also get rid of the "eventpoll" fd that is always there and not
			// interesting.
			filtered = append(filtered, target)
			continue
		}
		if prefix != "" && !strings.HasPrefix(target, prefix) {
			filtered = append(filtered, target)
			continue
		}
		out = append(out, n+"="+target)
	}
	out = append(out, fmt.Sprintf("(filtered: %s)", strings.Join(filtered, ", ")))
	return out
}
