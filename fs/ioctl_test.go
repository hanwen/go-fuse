// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"syscall"
	"testing"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fuse/ioctl"
)

type ioctlNode struct {
	Inode
}

var _ = (NodeIoctler)((*ioctlNode)(nil))

func (n *ioctlNode) Ioctl(ctx context.Context, f FileHandle, cmd ioctl.Command, arg uint64, input []byte, output []byte) (result int32, errno syscall.Errno) {
	for i := range output {
		output[i] = input[i] + 1
	}
	return 1515, 0
}

func TestIoctl(t *testing.T) {
	root := &ioctlNode{}
	mntDir, _ := testMount(t, root, nil)
	f, err := os.Open(mntDir)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	n := 42
	arg := bytes.Repeat([]byte{'x'}, n)

	cmd := ioctl.New(ioctl.READ|ioctl.WRITE,
		1, 1, uintptr(n))
	a, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(f.Fd()), uintptr(cmd), uintptr(unsafe.Pointer(&arg[0])))
	if errno != 0 {
		t.Fatal(errno)
	}
	if a != 1515 {
		t.Logf("got %d, want 1515", a)
	}

	if want := bytes.Repeat([]byte{'y'}, n); !reflect.DeepEqual(arg, want) {
		t.Logf("got %v, want %v", arg, want)
	}
}
