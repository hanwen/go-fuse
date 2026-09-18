// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nfs

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/willscott/go-nfs-client/nfs"
	"github.com/willscott/go-nfs-client/nfs/rpc"
	"golang.org/x/sys/unix"
)

// exportFsid returns a fsid that is random but fixed for the run's duration.
func exportFsid() string {
	// PID alone would risk colliding with a stale entry from an earlier
	// run that reused the same PID (common in short-lived containers);
	// mix in the current time too.
	return fmt.Sprintf("%d%d", os.Getpid(), time.Now().UnixNano()%1_000_000)
}

const rpcSetupHint = "start them first (needs root):\n" +
	"  - systemd (eg. Fedora/RHEL): systemctl enable --now rpcbind nfs-server\n" +
	"    (nfs-server starts rpc.mountd; needs an /etc/exports, even an empty one)\n" +
	"  - no systemd: rpcbind; rpc.mountd\n" +
	"  - verify: rpcinfo -p localhost   # should list mountd and nfs entries"

func requireReconnectTestEnv(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("test needs linux")
	}
	if os.Geteuid() != 0 {
		t.Skip("test needs root (exportfs, mounting NFS, /sys/fs/fuse/connections/*/abort)")
	}
	if _, err := os.Stat("/sys/fs/fuse/connections"); err != nil {
		t.Skipf("no /sys/fs/fuse/connections: %v", err)
	}
	if _, err := os.Stat("/proc/fs/nfsd"); err != nil {
		t.Skipf("nfsd not available (no /proc/fs/nfsd): %v", err)
	}
	for _, bin := range []string{"exportfs"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found: %v", bin, err)
		}
	}
}

// export manages one exportfs entry across the abort+remount in
// TestNFSExportReconnect. It omits no_subtree_check, so nfsd
// revalidates a reconnected file's ancestry, issuing ".." LOOKUPs.
type export struct {
	t    *testing.T
	spec string // "host:path", passed to exportfs
	fsid string
}

func newExport(t *testing.T, dir string) *export {
	t.Helper()
	e := &export{
		t:    t,
		spec: fmt.Sprintf("127.0.0.1:%s", dir),
		fsid: exportFsid(),
	}
	e.export()
	return e
}

func (e *export) export() {
	e.t.Helper()
	cmd := exec.Command("exportfs", "-i", "-o",
		"rw,insecure,no_root_squash,fsid="+e.fsid,
		e.spec)
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Skipf("exportfs failed: %v: %s\nrpcbind and/or rpc.mountd are probably not running - %s", err, out, rpcSetupHint)
	}
}

func (e *export) unexport() {
	e.t.Helper()
	if out, err := exec.Command("exportfs", "-u", e.spec).CombinedOutput(); err != nil {
		e.t.Logf("exportfs -u %s: %v: %s", e.spec, err, out)
	}
}

// dialNFS mounts dir over NFSv3 via the loopback interface.
func dialNFS(t *testing.T, dir string) (*nfs.Target, func()) {
	t.Helper()
	mnt, err := nfs.DialMount("127.0.0.1", 5*time.Second)
	if err != nil {
		t.Skipf("DialMount: %v\nrpcbind is probably not running - %s", err, rpcSetupHint)
	}
	auth := rpc.NewAuthUnix("go-fuse-nfs-test", 0, 0).Auth()
	target, err := mnt.Mount(dir, auth)
	if err != nil {
		mnt.Unmount()
		t.Skipf("NFS MNT of %s: %v\nrpc.mountd is probably not running - %s", dir, err, rpcSetupHint)
	}
	return target, func() {
		target.Close()
		mnt.Unmount()
	}
}

// readAll reads from f (an *nfs.File) into buf and returns the bytes
// actually read, tolerating an io.EOF that comes back alongside data.
func readAll(t *testing.T, f io.Reader, buf []byte) []byte {
	t.Helper()
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	return buf[:n]
}

// forceUnmount unmounts dir (previously served by srv), falling back to
// a lazy (MNT_DETACH) unmount if a plain one fails with EBUSY.
//
// We haven't found a reliable way to unmount without lazy unmount.
func forceUnmount(t *testing.T, srv *fuse.Server, dir, label string) {
	t.Helper()
	err := srv.Unmount()
	if err == nil {
		return
	}
	t.Logf("Unmount %s: %v; falling back to a lazy unmount", label, err)

	// Best-effort: make sure srv's Go-side resources aren't left running
	// against a connection that's about to be ripped out of the mount
	// table from under it. A no-op (logged, not fatal) if srv's
	// connection was already aborted by the caller, as it always is for
	// server A's mid-test replacement.
	if abortErr := abortConnection(dir); abortErr != nil {
		t.Logf("abort before lazy unmount: %v", abortErr)
	}

	bin, lookErr := exec.LookPath("fusermount3")
	if lookErr != nil {
		bin, lookErr = exec.LookPath("fusermount")
	}
	if lookErr != nil {
		t.Fatalf("Unmount %s: %v (and no fusermount binary for a lazy fallback: %v)", label, err, lookErr)
	}
	if out, lazyErr := exec.Command(bin, "-u", "-z", dir).CombinedOutput(); lazyErr != nil {
		t.Fatalf("Unmount %s: %v; lazy unmount also failed: %v: %s", label, err, lazyErr, out)
	}
}

// abortConnection severs the kernel FUSE connection backing dir,
func abortConnection(dir string) error {
	var st syscall.Stat_t
	if err := syscall.Stat(dir, &st); err != nil {
		return fmt.Errorf("Stat(%s): %w", dir, err)
	}
	connID := unix.Minor(uint64(st.Dev))
	abortPath := fmt.Sprintf("/sys/fs/fuse/connections/%d/abort", connID)
	if err := os.WriteFile(abortPath, nil, 0); err != nil {
		return fmt.Errorf("abort connection %d: %w", connID, err)
	}
	return nil
}

func TestNFSExportReconnect(t *testing.T) {
	requireReconnectTestEnv(t)

	const fileContent = "hello from before the reconnect"
	tree := newSynthTree([]byte(fileContent))

	dir := t.TempDir()
	serverA, err := mount(dir, tree)
	if err != nil {
		t.Fatalf("mount server A: %v", err)
	}
	t.Log("mounted server A")

	exp := newExport(t, dir)
	t.Log("exported dir over NFS")

	// current is whichever server is presently mounted at dir, so the
	// safety-net cleanup below (which must survive a t.Fatalf at any
	// point below) unmounts the right one regardless of how far the
	// test got.
	current := serverA
	defer func() {
		exp.unexport()
		forceUnmount(t, current, dir, "current server")
	}()

	target, cleanupClient := dialNFS(t, dir)
	defer cleanupClient()
	t.Log("mounted export via NFS client")

	f, err := target.Open(filepath.Join("a", "b", "file.txt"))
	if err != nil {
		t.Fatalf("Open(a/b/file.txt): %v", err)
	}
	defer f.Close()
	t.Log("opened a/b/file.txt via NFS")

	if got := string(readAll(t, f, make([]byte, 64))); got != fileContent {
		t.Fatalf("initial Read = %q, want %q", got, fileContent)
	}
	t.Log("initial read via NFS OK")

	_, bFH, err := target.Lookup(filepath.Join("a", "b"))
	if err != nil {
		t.Fatalf("Lookup(a/b): %v", err)
	}

	exp.unexport()
	if err := abortConnection(dir); err != nil {
		t.Fatalf("abortConnection: %v", err)
	}
	forceUnmount(t, serverA, dir, "server A")
	t.Log("aborted connection and unmounted server A")

	serverB, err := mount(dir, tree)
	if err != nil {
		t.Fatalf("mount server B: %v", err)
	}
	current = serverB
	t.Log("mounted server B (fresh identity table, same backing tree)")

	exp.export()
	t.Log("re-exported dir over NFS (now serving server B)")

	// Re-read through the filehandle opened against server A. The
	// kernel's cached dentry for it is gone (server A's mount, and its
	// nodeid, no longer exist), so nfsd must reconnect the filehandle -
	// which, for a FUSE-backed export, is the LOOKUP(nodeid, ".") this
	// example's NodeLookupNoder implementation exists to answer.
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek after remount: %v", err)
	}
	if got := string(readAll(t, f, make([]byte, 64))); got != fileContent {
		t.Fatalf("Read after reconnect = %q, want %q", got, fileContent)
	}
	t.Log("read after reconnect OK - filehandle survived the restart")

	// Reconnecting a/b resolves its parent "a" via LookupParent; the
	// GETATTR then fails if "a" was not registered.
	bAttr, err := target.GetAttr(bFH)
	if err != nil {
		t.Fatalf("GetAttr(a/b) after reconnect: %v", err)
	}
	if !bAttr.IsDir() {
		t.Fatalf("GetAttr(a/b) after reconnect: mode %v, want a directory", bAttr.Mode())
	}
}
