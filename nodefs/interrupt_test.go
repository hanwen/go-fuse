// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type interruptRoot struct {
	DefaultOperations
}

type interruptOps struct {
	DefaultOperations
	Data []byte
}

func (r *interruptRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {
	if name != "file" {
		return nil, fuse.ENOENT
	}
	ch := InodeOf(r).NewInode(&interruptOps{
		DefaultOperations{},
		bytes.Repeat([]byte{42}, 1024),
	}, fuse.S_IFREG, FileID{
		Ino: 2,
		Gen: 1})

	out.Size = 1024
	out.Mode = fuse.S_IFREG | 0644

	return ch, fuse.OK
}

func (o *interruptOps) GetAttr(ctx context.Context, f File, out *fuse.AttrOut) fuse.Status {
	out.Mode = fuse.S_IFREG | 0644
	out.Size = uint64(len(o.Data))
	return fuse.OK
}

type interruptFile struct {
	DefaultFile
}

func (f *interruptFile) Flush(ctx context.Context) fuse.Status {
	return fuse.OK
}

func (o *interruptOps) Open(ctx context.Context, flags uint32) (File, uint32, fuse.Status) {
	return &interruptFile{}, 0, fuse.OK
}

func (o *interruptOps) Read(ctx context.Context, f File, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	time.Sleep(100 * time.Millisecond)
	end := int(off) + len(dest)
	if end > len(o.Data) {
		end = len(o.Data)
	}

	return fuse.ReadResultData(o.Data[off:end]), fuse.OK
}

// This currently doesn't test functionality, but is useful to investigate how
// INTERRUPT opcodes are handled.
func TestInterrupt(t *testing.T) {
	mntDir := testutil.TempDir()
	defer os.Remove(mntDir)
	loopback := &interruptRoot{DefaultOperations{}}

	_ = time.Second
	oneSec := time.Second
	rawFS := NewNodeFS(loopback, &Options{
		Debug: testutil.VerboseTest(),

		// NOSUBMIT - should run all tests without cache too
		EntryTimeout: &oneSec,
		AttrTimeout:  &oneSec,
	})

	server, err := fuse.NewServer(rawFS, mntDir,
		&fuse.MountOptions{
			Debug: testutil.VerboseTest(),
		})
	if err != nil {
		t.Fatal(err)
	}

	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	cmd := exec.Command("cat", mntDir+"/file")
	if err := cmd.Start(); err != nil {
		t.Fatalf("run %v: %v", cmd, err)
	}

	time.Sleep(10 * time.Millisecond)
	log.Println("killing subprocess")
	if err := cmd.Process.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
}
