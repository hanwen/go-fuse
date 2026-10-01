// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

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
// stack end-to-end. With CAP_SECURITY_CTX, the kernel sends a request
// extension for every create-type op, even without an LSM.
func TestPosixtest(t *testing.T) {
	posixBin := buildStaticPosixtest(t)

	orig := t.TempDir()

	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	var logBuf lockedBuffer
	opts := &fs.Options{}
	opts.Debug = true
	opts.Logger = log.New(&logBuf, "", 0)
	opts.MountOptions.Logger = opts.Logger
	opts.MountOptions.ExtraCapabilities = fuse.CAP_SECURITY_CTX
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("debug log:\n%s", logBuf.String())
		}
	})

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

/posixtest.test -posixdir=/mnt -test.run TestAll -test.skip TestAll/DirectIO -test.v \
    > /mnt/test_output.txt 2>&1
echo $? > /mnt/test_exit.txt

# posixtest has no MKNOD.
mkfifo /mnt/fifo

ls /mnt/killme.txt
reboot -n -f
`

	extraFiles := map[string]string{
		"/posixtest.test": posixBin,
	}
	if err := mkinitRam(ramdisk, testAssets.busybox, testAssets.modules, []byte(initScript), extraFiles); err != nil {
		t.Fatal(err)
	}

	cmd := newQemuCmd(sockpath, ramdisk)
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
		m := regexp.MustCompile(`--- FAIL: *([^ ]*)\s\([0-9.]*s\)`).FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		t.Errorf("posixtest sub-test failed: %s", name)
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

	if fi, err := os.Lstat(orig + "/fifo"); err != nil {
		t.Errorf("Lstat: %v", err)
	} else if fi.Mode().Type() != os.ModeNamedPipe {
		t.Errorf("Lstat(fifo): got type %v", fi.Mode().Type())
	}

	debugLog := logBuf.String()
	if !strings.Contains(debugLog, "SECURITY_CTX") {
		t.Log("guest kernel does not support CAP_SECURITY_CTX")
		return
	}
	for _, op := range []string{"SYMLINK", "MKDIR", "MKNOD", "CREATE"} {
		found := false
		for _, l := range strings.Split(debugLog, "\n") {
			if strings.HasPrefix(l, "rx ") && strings.Contains(l, ": "+op+" ") {
				found = true
				if !strings.Contains(l, " ext{") {
					t.Errorf("%s request has no extension: %s", op, l)
					break
				}
			}
		}
		if !found {
			t.Errorf("no %s request in log", op)
		}
	}
}
