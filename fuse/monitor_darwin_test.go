// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"
)

// monitorTestRoot is an empty directory. macFUSE stats the root before the
// mount completes, so GetAttr has to work; the rest of the mount, including
// the poll hack, is answered inside the protocol server.
type monitorTestRoot struct {
	RawFileSystem
}

func (fs *monitorTestRoot) GetAttr(ctx context.Context, input *GetAttrIn, out *AttrOut) Status {
	out.Mode = S_IFDIR | 0755
	out.Nlink = 2
	return OK
}

// TestMonitorNotAnswered checks that the MONITOR requests macFUSE sends while
// mounting are left unanswered. A reply goes to /dev/macfuseN for a request
// the kernel is not tracking: the write fails with ENOENT and is logged.
//
// Debug is on so that a MONITOR request leaves a trace even when its reply is
// suppressed; without it, a kernel that sends no MONITOR at all would look the
// same as working suppression.
func TestMonitorNotAnswered(t *testing.T) {
	var logBuf bytes.Buffer
	opts := MountOptions{
		Debug:  true,
		Logger: log.New(&logBuf, "", 0),
	}
	mnt := t.TempDir()
	srv, err := NewServer(&monitorTestRoot{NewDefaultRawFileSystem()}, mnt, &opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mnt); err != nil {
		t.Errorf("Stat: %v", err)
	}
	if err := srv.Unmount(); err != nil {
		t.Fatal(err)
	}
	srv.Wait()

	logged := logBuf.String()
	// "rx N: MONITOR nX" is logged before the request is dispatched.
	if !strings.Contains(logged, ": MONITOR n") {
		t.Skip("kernel sent no MONITOR request")
	}
	t.Logf("received %d MONITOR requests", strings.Count(logged, ": MONITOR n"))
	// "opcode: MONITOR" means a reply was written, and the write failed.
	if strings.Contains(logged, "opcode: MONITOR") {
		t.Error("MONITOR was answered; the reply to /dev/macfuseN failed with ENOENT")
	}
}
