// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package test

import (
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"testing"
)

func TestFlock(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	tc.WriteFile(tc.origFile, []byte(contents), 0700)

	f, err := os.OpenFile(tc.mountFile, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", tc.mountFile, err)
	}
	defer f.Close()

	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Errorf("Flock returned: %v", err)
		return
	}

	if out, err := runExternalFlock(tc.mountFile); err != nil {
		re := regexp.MustCompile(`flock_test.go:\d+: (.*?)$`) // don't judge me
		t.Errorf("runExternalFlock(%q): %s", tc.mountFile, re.Find(out))
	}
}

func TestFlockExternal(t *testing.T) {
	fname := os.Getenv("LOCKED_FILE")
	if fname == "" {
		t.SkipNow()
	}

	f, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != syscall.EAGAIN {
		t.Errorf("expexpected EAGAIN, got (%+v):  %v)", err, err)
	}
}

func runExternalFlock(fname string) ([]byte, error) {
	cmd := exec.Command(os.Args[0], "-test.run=TestFlockExternal", "-test.timeout=1s")

	cmd.Env = append(cmd.Env, "LOCKED_FILE="+fname)
	return cmd.CombinedOutput()
}
