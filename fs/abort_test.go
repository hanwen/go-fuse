// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"
)

type hangingRootNode struct {
	Inode
	openCalled chan struct{}

	mu       sync.Mutex
	canceled bool
}

func (n *hangingRootNode) OpendirHandle(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	close(n.openCalled)
	select {
	case <-ctx.Done():
		n.mu.Lock()
		n.canceled = true
		n.mu.Unlock()
		return nil, 0, syscall.EINTR
	}
}

func TestAbort(t *testing.T) {
	hr := &hangingRootNode{
		openCalled: make(chan struct{}, 0),
	}
	dir, srv := testMount(t, hr, nil)

	var st syscall.Stat_t
	if err := syscall.Lstat(dir, &st); err != nil {
		t.Fatal(err)
	}

	connectionID := st.Dev

	done := make(chan error, 1)
	go func() {
		_, err := syscall.Open(dir, syscall.O_DIRECTORY, 0)
		done <- err
		close(done)
	}()

	<-hr.openCalled

	if err := os.WriteFile(fmt.Sprintf("/sys/fs/fuse/connections/%d/abort", connectionID), []byte{}, 0); err != nil {
		t.Fatal(err)
	}

	if err := <-done; err == nil {
		t.Error("opendir should have failed")
	}

	// Wait until read loops have exited, so we can be sure
	// cancelation was propagated.
	srv.Unmount()

	hr.mu.Lock()
	defer hr.mu.Unlock()
	if !hr.canceled {
		t.Error("should have been canceled.")
	}
}
