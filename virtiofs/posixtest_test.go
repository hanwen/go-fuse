// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
)

// buildStaticPosixtest compiles the posixtest package into a static test binary
// and returns its path.  The binary is placed in a temp file owned by the
// caller (use t.Cleanup or defer os.Remove).
func buildStaticPosixtest(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/posixtest.test"
	cmd := exec.Command("go", "test", "-c",
		"-o", bin,
		"github.com/hanwen/go-fuse/v2/posixtest",
	)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build posixtest.test: %v\n%s", err, out)
	}
	return bin
}

// TestPosixtest runs the posixtest suite inside a QEMU VM against a virtiofs
// mount backed by a host loopback, exercising the full virtiofs + go-fuse
// stack end-to-end.
func TestPosixtest(t *testing.T) {
	posixBin := buildStaticPosixtest(t)

	orig := t.TempDir()

	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	opts := &fs.Options{}
	opts.Logger = log.Default()
	opts.MountOptions.Logger = opts.Logger
	//	opts.Debug = true

	r := &killNotifyRoot{
		LoopbackNode: root.(*fs.LoopbackNode),
	}
	r.notify = sync.NewCond(&r.mu)

	rawFS := fs.NewNodeFS(r, opts)

	tmpDir := t.TempDir()
	sockpath := tmpDir + "/virtiofs.socket"
	ramdisk := tmpDir + "/initrd.cpio.gz"

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

mkdir -p /mnt/tmp

/posixtest.test -posixdir=/mnt -test.run TestAll -test.v \
    > /mnt/test_output.txt 2>&1
echo $? > /mnt/test_exit.txt

ls /mnt/killme.txt
reboot -n -f
`

	extraFiles := map[string]string{
		"/posixtest.test": posixBin,
	}
	if err := mkinitRam(ramdisk, testAssets.busybox, testAssets.modules, []byte(initScript), extraFiles); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(testAssets.qemuBin,
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

	go func() {
		r.mu.Lock()
		for !r.seenKill {
			r.notify.Wait()
		}
		r.mu.Unlock()
		log.Println("killing qemu..")
		cmd.Process.Kill()
	}()
	cmd.Wait()

	// Parse and report test output.
	outputBytes, err := os.ReadFile(orig + "/test_output.txt")
	if err != nil {
		t.Fatalf("test_output.txt not found — guest may not have run: %v", err)
	}
	exitBytes, _ := os.ReadFile(orig + "/test_exit.txt")
	exitCode := strings.TrimSpace(string(exitBytes))

	output := string(outputBytes)
	t.Logf("posixtest output:\n%s", output)

	// Report individual sub-test failures into Go's testing framework.
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- FAIL:") {
			name := strings.Fields(line)[2]
			t.Errorf("posixtest sub-test failed: %s", name)
		}
	}
	if exitCode != "0" && exitCode != "" {
		t.Errorf("posixtest.test exited with code %s", exitCode)
	}

	// Ensure the guest actually ran by checking the sentinel file exists.
	if _, err := os.Stat(orig + "/test_exit.txt"); os.IsNotExist(err) {
		t.Error("test_exit.txt missing — guest did not complete")
	}

	// Verify that the killNotifyRoot saw the killme.txt lookup, confirming
	// the guest reached the end of the init script.
	r.mu.Lock()
	seenKill := r.seenKill
	r.mu.Unlock()
	if !seenKill {
		t.Error("guest did not signal completion (killme.txt not looked up)")
	}

}
