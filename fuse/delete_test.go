package fuse

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

type flipNode struct {
	*memNode
	ok chan int
}

func (f *flipNode) GetAttr(out *Attr, file File, c *Context) Status {
	select {
	case <-f.ok:
		// use a status that is easily recognizable.
		return Status(syscall.EXDEV)
	default:
	}
	return f.memNode.GetAttr(out, file, c)
}

func TestDeleteNotify(t *testing.T) {
	dir, err := ioutil.TempDir("","")
	if err != nil {
		t.Fatalf("TempDir failed %v", err)
	}
	defer os.RemoveAll(dir)
	fs := NewMemNodeFs(dir + "/backing")
	conn := NewFileSystemConnector(fs,
		&FileSystemOptions{PortableInodes: true})
	state := NewMountState(conn)
	mnt := dir + "/mnt"
	err = os.Mkdir(mnt, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = state.Mount(mnt, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Debug = VerboseTest()
	go state.Loop()
	defer state.Unmount()

	_, code := fs.Root().Mkdir("testdir", 0755, nil)
	if !code.Ok() {
		t.Fatal(code)
	}

	ch := fs.Root().Inode().RmChild("testdir")
	ch.FsNode().(*memNode).SetInode(nil)
	flip := flipNode{
		memNode: ch.FsNode().(*memNode),
		ok: make(chan int),
	}
	newCh := fs.Root().Inode().New(true, &flip)
	fs.Root().Inode().AddChild("testdir", newCh)

	err = ioutil.WriteFile(mnt + "/testdir/testfile", []byte{42}, 0644)
	if err != nil {
		t.Fatal(err)
	}
	buf := bytes.Buffer{}
	cmd := exec.Command("/usr/bin/tail", "-f", "testfile")
	cmd.Dir = mnt + "/testdir"
	cmd.Stdin = &buf
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		cmd.Process.Kill()
		time.Sleep(100*time.Millisecond)
	}()

	// Wait until tail opened the file.
	time.Sleep(100*time.Millisecond)
	err = os.Remove(mnt + "/testdir/testfile")
	if err != nil {
		t.Fatal(err)
	}

	// Simulate deletion+mkdir coming from the network
	close(flip.ok)
	oldCh := fs.Root().Inode().RmChild("testdir")
	_, code = fs.Root().Inode().FsNode().Mkdir("testdir", 0755, nil)
	if !code.Ok() {
		t.Fatal("mkdir status", code)
	}
	conn.DeleteNotify(fs.Root().Inode(), oldCh, "testdir")

	_, err = os.Lstat(mnt + "/testdir")
	if err != nil {
		t.Fatalf("lstat after del + mkdir failed: %v", err)
	}
}
