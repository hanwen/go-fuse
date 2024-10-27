// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

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
	mu        sync.Mutex
	seenKill  bool
	seenStart bool
	notify    *sync.Cond
}

var _ = (fs.NodeCreater)((*killNotifyRoot)(nil))

func (r *killNotifyRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "start.txt" {
		r.mu.Lock()
		r.seenStart = true
		r.notify.Broadcast()
		r.mu.Unlock()
	}

	if name == "killme.txt" {
		log.Println("saw killme.txt")
		r.mu.Lock()
		r.seenKill = true
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

	initScript := "#!/bin/sh\n" +
		"mount -t proc none /proc\n" +
		"mount -t sysfs none /sys\n" +
		moduleInsmodLines(testAssets.modules) +
		`
echo '****'
echo "init started; Boot took $(cut -d' ' -f1 /proc/uptime) seconds"
echo '****'
set -x

mount -t virtiofs myfs /mnt

ls -1 /mnt/start.txt
cp /mnt/file.txt /mnt/new.txt

# leave /mnt to enable unmount
(cd /mnt && ls -1 > files.txt)

sleep 1
ls /mnt/killme.txt

reboot -n -f
`

	if err := mkinitRam(ramdisk, testAssets.busybox, testAssets.modules, []byte(initScript)); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(ramdisk)

	cmd := exec.Command("qemu-system-x86_64",
		"-M", "pc", "-m", "4G", "-cpu", "host", "-smp", "2",
		"-enable-kvm",
		"-chardev", "socket,id=char0,path="+sockpath,
		"-device", "vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=myfs",
		"-object", "memory-backend-file,id=mem,size=4G,mem-path=/dev/shm,share=on",
		"-numa", "node,memdev=mem",
		"-kernel", testAssets.kernel,
		"-initrd", ramdisk,
		"-nographic",
		"-no-reboot",
		"-append", "console=ttyS0",
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println("running", cmd.Args)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	r.mu.Lock()
	for !r.seenStart {
		r.notify.Wait()
	}
	r.mu.Unlock()
	/*
		time.Sleep(10 * time.Millisecond)
		if errno := r.NotifyEntry("orig.txt"); errno != 0 {
			t.Logf("NotifyEntry: %v", errno)
		}
	*/
	go func() {
		r.mu.Lock()
		for !r.seenKill {
			r.notify.Wait()
		}
		r.mu.Unlock()
		log.Println("killing qemu..")
		cmd.Process.Kill()
		log.Println("killed")
	}()
	cmd.Wait()

	if readback, err := os.ReadFile(orig + "/new.txt"); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(readback, content) {
		t.Errorf("got %q, want %q", readback, content)
	}

	got, err := os.ReadFile(orig + "/files.txt")
	if err != nil {
		t.Fatal("ReadFile: ", err)
	}
	want := []byte(`file.txt
files.txt
new.txt
`)
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}
