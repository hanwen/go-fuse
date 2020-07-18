// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func waitMount(mnt string) error {
	for i := 0; i < 100; i++ {
		err := exec.Command("mountpoint", mnt).Run()
		if err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return syscall.ETIMEDOUT
}

func TestMountRo(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(dir, "orig")
	mnt := filepath.Join(dir, "mnt")
	if err := os.Mkdir(orig, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(mnt, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("./loopback", "-ro", mnt, orig)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}
	if err := waitMount(mnt); err != nil {
		t.Fatal(err)
	}
	exec.Command("fusermount", "-u", mnt).Run()
	if err := cmd.Wait(); err != nil {
		t.Error(err)
	}
}
