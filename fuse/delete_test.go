package fuse

import (
	"bytes"
	"io/ioutil"
	"log"
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
		log.Println("killing process...")
		cmd.Process.Kill()
		cmd.Wait()
		log.Println("waited")
		time.Sleep(100*time.Millisecond)
	}()

	// Wait until we see the subprocess moving.
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := os.Lstat(mnt + "/testdir/testfile")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout; process did not start?")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Simulate deletion+mkdir coming from the network
	close(flip.ok)
	fs.Root().Inode().RmChild("testdir")
	_, code = fs.Root().Inode().FsNode().Mkdir("testdir", 0755, nil)
	if !code.Ok() {
		t.Fatal("mkdir status", code)
	}
	conn.EntryNotify(fs.Root().Inode(), "testdir")

	fi, err := os.Lstat(mnt + "/testdir")
	log.Println("lsta", fi, err)
	if err != nil {
		t.Fatalf("lstat after del + mkdir failed: %v", err)
	}
	t.Log(fi)
}
