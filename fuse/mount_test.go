package fuse

import (
	"os"
	"testing"
	"time"
	"path/filepath"
	"io/ioutil"
)

func TestMountOnExisting(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	err := os.Mkdir(ts.mnt+"/mnt", 0777)
	CheckSuccess(err)
	nfs := &DefaultNodeFileSystem{}
	code := ts.connector.Mount(ts.rootNode(), "mnt", nfs, nil)
	if code != EBUSY {
		t.Fatal("expect EBUSY:", code)
	}

	err = os.Remove(ts.mnt + "/mnt")
	CheckSuccess(err)
	code = ts.connector.Mount(ts.rootNode(), "mnt", nfs, nil)
	if !code.Ok() {
		t.Fatal("expect OK:", code)
	}

	ts.pathFs.Unmount("mnt")
}

func TestMountRename(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fs := NewPathNodeFs(NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}
	err := os.Rename(ts.mnt+"/mnt", ts.mnt+"/foobar")
	if OsErrorToErrno(err) != EBUSY {
		t.Fatal("rename mount point should fail with EBUSY:", err)
	}
	ts.pathFs.Unmount("mnt")
}

func TestMountReaddir(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fs := NewPathNodeFs(NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}

	entries, err := ioutil.ReadDir(ts.mnt)
	CheckSuccess(err)
	if len(entries) != 1 || entries[0].Name != "mnt" {
		t.Error("wrong readdir result", entries)
	}
	ts.pathFs.Unmount("mnt")
}

func TestRecursiveMount(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	err := ioutil.WriteFile(ts.orig+"/hello.txt", []byte("blabla"), 0644)
	CheckSuccess(err)

	fs := NewPathNodeFs(NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}

	submnt := ts.mnt + "/mnt"
	_, err = os.Lstat(submnt)
	CheckSuccess(err)
	_, err = os.Lstat(filepath.Join(submnt, "hello.txt"))
	CheckSuccess(err)

	f, err := os.Open(filepath.Join(submnt, "hello.txt"))
	CheckSuccess(err)
	t.Log("Attempting unmount, should fail")
	code = ts.pathFs.Unmount("mnt")
	if code != EBUSY {
		t.Error("expect EBUSY")
	}

	f.Close()

	t.Log("Waiting for kernel to flush file-close to fuse...")
	time.Sleep(1.5e9 * testTtl)

	t.Log("Attempting unmount, should succeed")
	code = ts.pathFs.Unmount("mnt")
	if code != OK {
		t.Error("umount failed.", code)
	}
}

func TestDeletedUnmount(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	submnt := filepath.Join(ts.mnt, "mnt")
	pfs2 := NewPathNodeFs(NewLoopbackFileSystem(ts.orig), nil)
	code := ts.connector.Mount(ts.rootNode(), "mnt", pfs2, nil)
	if !code.Ok() {
		t.Fatal("Mount error", code)
	}
	f, err := os.Create(filepath.Join(submnt, "hello.txt"))
	CheckSuccess(err)

	t.Log("Removing")
	err = os.Remove(filepath.Join(submnt, "hello.txt"))
	CheckSuccess(err)

	t.Log("Removing")
	_, err = f.Write([]byte("bla"))
	CheckSuccess(err)

	code = ts.pathFs.Unmount("mnt")
	if code != EBUSY {
		t.Error("expect EBUSY for unmount with open files", code)
	}

	f.Close()
	time.Sleep(1.5e9 * testTtl)
	code = ts.pathFs.Unmount("mnt")
	if !code.Ok() {
		t.Error("should succeed", code)
	}
}
