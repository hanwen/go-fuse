package fuse

import (
	"log"
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
	code := ts.connector.Mount("/mnt", nfs, nil)
	if code != EBUSY {
		t.Fatal("expect EBUSY:", code)
	}

	err = os.Remove(ts.mnt + "/mnt")
	CheckSuccess(err)
	code = ts.connector.Mount("/mnt", nfs, nil)
	if !code.Ok() {
		t.Fatal("expect OK:", code)
	}
}

func TestUnmountNoExist(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	code := ts.connector.Unmount("/doesnotexist")
	if code != EINVAL {
		t.Fatal("expect EINVAL", code)
	}
}

func TestMountRename(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fs := NewPathNodeFs(NewLoopbackFileSystem(ts.orig))
	code := ts.connector.Mount("/mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}
	err := os.Rename(ts.mnt+"/mnt", ts.mnt+"/foobar")
	if OsErrorToErrno(err) != EBUSY {
		t.Fatal("rename mount point should fail with EBUSY:", err)
	}
}

func TestMountReaddir(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fs := NewPathNodeFs(NewLoopbackFileSystem(ts.orig))
	code := ts.connector.Mount("/mnt", fs, nil)
	if !code.Ok() {
		t.Fatal("mount should succeed")
	}

	entries, err := ioutil.ReadDir(ts.mnt)
	CheckSuccess(err)
	if len(entries) != 1 || entries[0].Name != "mnt" {
		t.Error("wrong readdir result", entries)
	}
}

func TestRecursiveMount(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	err := ioutil.WriteFile(ts.orig+"/hello.txt", []byte("blabla"), 0644)
	CheckSuccess(err)

	fs := NewPathNodeFs(NewLoopbackFileSystem(ts.orig))
	code := ts.connector.Mount("/mnt", fs, nil)
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
	log.Println("Attempting unmount, should fail")
	code = ts.connector.Unmount("/mnt")
	if code != EBUSY {
		t.Error("expect EBUSY")
	}

	f.Close()

	log.Println("Waiting for kernel to flush file-close to fuse...")
	time.Sleep(1.5e9 * testTtl)

	log.Println("Attempting unmount, should succeed")
	code = ts.connector.Unmount("/mnt")
	if code != OK {
		t.Error("umount failed.", code)
	}
}

func TestDeletedUnmount(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	submnt := filepath.Join(ts.mnt, "mnt")
	pfs2 := NewPathNodeFs(NewLoopbackFileSystem(ts.orig))
	code := ts.connector.Mount("/mnt", pfs2, nil)
	if !code.Ok() {
		t.Fatal("Mount error", code)
	}
	f, err := os.Create(filepath.Join(submnt, "hello.txt"))
	CheckSuccess(err)

	log.Println("Removing")
	err = os.Remove(filepath.Join(submnt, "hello.txt"))
	CheckSuccess(err)

	log.Println("Removing")
	_, err = f.Write([]byte("bla"))
	CheckSuccess(err)

	code = ts.connector.Unmount("/mnt")
	if code != EBUSY {
		t.Error("expect EBUSY for unmount with open files", code)
	}

	f.Close()
	time.Sleep(1.5e9 * testTtl)
	code = ts.connector.Unmount("/mnt")
	if !code.Ok() {
		t.Error("should succeed", code)
	}
}
