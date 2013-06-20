package test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

func TestMountOnExisting(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	err := os.Mkdir(ts.mnt+"/mnt", 0777)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	nfs := &fuse.DefaultNodeFileSystem{}
	code := ts.connector.Mount(ts.rootNode(), "mnt", nfs, nil)
	if code != fuse.EBUSY {
		t.Fatal("expect EBUSY:", code)
	}

	err = os.Remove(ts.mnt + "/mnt")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	code = ts.connector.Mount(ts.rootNode(), "mnt", nfs, nil)
	if !code.Ok() {
		t.Fatal("expect OK:", code)
	}

	code = ts.pathFs.Unmount("mnt")
	if !code.Ok() {
		t.Errorf("Unmount failed: %v", code)
	}
}

func TestMountRename(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fs := fuse.NewPathNodeFs(fuse.NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}
	err := os.Rename(ts.mnt+"/mnt", ts.mnt+"/foobar")
	if fuse.ToStatus(err) != fuse.EBUSY {
		t.Fatal("rename mount point should fail with EBUSY:", err)
	}
	ts.pathFs.Unmount("mnt")
}

func TestMountReaddir(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fs := fuse.NewPathNodeFs(fuse.NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}

	entries, err := ioutil.ReadDir(ts.mnt)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "mnt" {
		t.Error("wrong readdir result", entries)
	}
	ts.pathFs.Unmount("mnt")
}

func TestRecursiveMount(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	err := ioutil.WriteFile(ts.orig+"/hello.txt", []byte("blabla"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fs := fuse.NewPathNodeFs(fuse.NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}

	submnt := ts.mnt + "/mnt"
	_, err = os.Lstat(submnt)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	_, err = os.Lstat(filepath.Join(submnt, "hello.txt"))
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}

	f, err := os.Open(filepath.Join(submnt, "hello.txt"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Log("Attempting unmount, should fail")
	code = ts.pathFs.Unmount("mnt")
	if code != fuse.EBUSY {
		t.Error("expect EBUSY")
	}

	f.Close()
	t.Log("Waiting for kernel to flush file-close to fuse...")
	time.Sleep(testTtl)

	t.Log("Attempting unmount, should succeed")
	code = ts.pathFs.Unmount("mnt")
	if code != fuse.OK {
		t.Error("umount failed.", code)
	}
}

func TestDeletedUnmount(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	submnt := filepath.Join(ts.mnt, "mnt")
	pfs2 := fuse.NewPathNodeFs(fuse.NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", pfs2, nil)
	if !code.Ok() {
		t.Fatal("Mount error", code)
	}
	f, err := os.Create(filepath.Join(submnt, "hello.txt"))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	t.Log("Removing")
	err = os.Remove(filepath.Join(submnt, "hello.txt"))
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	t.Log("Removing")
	_, err = f.Write([]byte("bla"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	code = ts.pathFs.Unmount("mnt")
	if code != fuse.EBUSY {
		t.Error("expect EBUSY for unmount with open files", code)
	}

	f.Close()
	time.Sleep((3 * testTtl) / 2)
	code = ts.pathFs.Unmount("mnt")
	if !code.Ok() {
		t.Error("should succeed", code)
	}
}
