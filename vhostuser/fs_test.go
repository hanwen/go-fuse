// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// This is for the guest to signal it's finished. This is because I am
// unable to make QEMU exit when the guest calls poweroff.
type killNotifyRoot struct {
	*fs.LoopbackNode
	mu     sync.Mutex
	seen   bool
	notify *sync.Cond
}

var _ = (fs.NodeCreater)((*killNotifyRoot)(nil))

func (r *killNotifyRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "killme.txt" {
		log.Println("saw killme.txt")
		r.mu.Lock()
		r.seen = true
		r.notify.Broadcast()
		r.mu.Unlock()
	}

	return r.LoopbackNode.Lookup(ctx, name, out)
}

func TestBasic(t *testing.T) {
	orig := t.TempDir()

	content := []byte("hello world\n")
	if err := os.WriteFile(orig+"/file.txt", content, 0666); err != nil {
		t.Errorf("WriteFile: %v", err)
	}
	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	opts := &fs.Options{}
	opts.Debug = true
	opts.Logger = log.Default()
	opts.MountOptions.Logger = opts.Logger

	r := &killNotifyRoot{
		LoopbackNode: root.(*fs.LoopbackNode),
	}

	r.notify = sync.NewCond(&r.mu)

	rawFS := fs.NewNodeFS(r, opts)

	bindir := os.Getenv("HOME") + "/.cache/go-fuse-virtiofs"

	tf, err := os.CreateTemp("", "vhostuser")
	if err != nil {
		t.Fatal(err)
	}
	sockpath := tf.Name() + ".socket"
	ramdisk := tf.Name() + ".cpio.gz"
	tf.Close()
	defer os.Remove(sockpath)
	os.Remove(sockpath)

	go ServeFS(sockpath, rawFS, &opts.MountOptions)

	busybox := os.Getenv("BUSYBOX")
	if busybox == "" {
		t.Skip("must set $BUSYBOX")
	}
	kernel := os.Getenv("KERNEL")
	if kernel == "" {
		/* compile the kernel with support for

		   bzip compression
		   PCI
		   virtio
		   virtio_fs
		*/
		t.Skip("must set $KERNEL")
	}

	if err := mkinitRam(ramdisk, busybox, []byte(`#!/bin/sh 
mount -t proc none /proc
mount -t sysfs none /sys

echo '****'
echo "init started; Boot took $(cut -d' ' -f1 /proc/uptime) seconds"
echo '****'
set -x

mount -t virtiofs myfs /mnt

cp /mnt/file.txt /mnt/new.txt

# leave /mnt to enable unmount
(cd /mnt && ls -1 > files.txt)
ls /mnt/killme.txt
#umount /mnt 

# doesn't work?
reboot -n -f 
		`)); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(ramdisk)

	cmd := exec.Command("qemu-system-x86_64",
		"-M", "pc", "-m", "4G", "-cpu", "host", "-smp", "2",
		"-enable-kvm",
		// to create the communications socket
		"-chardev", "socket,id=char0,path="+sockpath,

		// instantiate the device
		"-device", "vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=myfs",

		// force use of memory sharable with virtiofsd.
		"-object", "memory-backend-file,id=mem,size=4G,mem-path=/dev/shm,share=on", "-numa", "node,memdev=mem",

		"-kernel", bindir+"/bzImage",
		"-initrd", ramdisk,
		"-nographic",
		"-no-reboot",
		"-append",
		"console=ttyS0",
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println("running", cmd.Args)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	go func() {
		r.mu.Lock()
		for !r.seen {
			r.notify.Wait()
		}
		r.mu.Unlock()
		cmd.Process.Kill()
	}()
	cmd.Wait()

	if readback, err := os.ReadFile(orig + "/new.txt"); err != nil {
		t.Fatal(err)
	} else if bytes.Compare(readback, content) != 0 {
		t.Errorf("bytes.Compare != 0, got %q, want %q", readback, content)
	}

	got, err := os.ReadFile(orig + "/files.txt")
	if err != nil {
		t.Fatal("ReadFile: ", err)
	}
	want := []byte(`file.txt
files.txt
new.txt
`)
	if bytes.Compare(got, want) != 0 {
		t.Errorf("got %q want %q", got, want)
	}
}
