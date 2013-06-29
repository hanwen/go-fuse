package nodefs

import (
	"os"
	"os/exec"
	"io/ioutil"
	"time"
	"testing"
	"path/filepath"
	"syscall"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/raw"
)

type testFs struct {
	FileSystem
	root Node
}

type testNode struct {
	Node
}

type testFile struct {
	File
}

func setupInterruptTest(t *testing.T) (dir string, clean func()) {
	tmp, err := ioutil.TempDir("", "go-fuse-interrupt_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}

	fs := &testFs{ FileSystem: NewDefaultFileSystem(), root: NewDefaultNode() }

	fsServer, _, err := MountFileSystem(tmp, fs, nil)
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	fsServer.SetDebug(fuse.VerboseTest())

	go fsServer.Serve()

	return tmp, func() {
		err := fsServer.Unmount()
		if err != nil {
			t.Fatalf("Unmount failed: %v", err)
		}
		os.RemoveAll(tmp)
	}
}

func (fs *testFs) Root() Node {
	return fs.root
}

func (fs *testFs) OnMount(conn *FileSystemConnector) {
	node := &testNode{ NewDefaultNode() }
	rino := fs.root.Inode()
	nino := rino.New(false, node)
	rino.AddChild("test", nino)
}

func (n *testNode) Open(flags uint32, context *fuse.Context) (File, fuse.Status) {
	return &WithFlags{ &testFile{ NewDefaultFile() }, "test", raw.FOPEN_DIRECT_IO, 0 }, fuse.OK
}

func (fh *testFile) Read(dest []byte, off int64, context *fuse.Context) (fuse.ReadResult, fuse.Status) {
	if off != 0 {
		return &fuse.ReadResultData{ []byte{} }, fuse.OK
	}
	select {
	case <- context.Interrupted:
		println("Interrupted")
		return &fuse.ReadResultData{ []byte{ '1' } }, fuse.Status(syscall.EINTR)
	case <- time.After(20 * time.Second):
		return &fuse.ReadResultData{ []byte{ '1' } }, fuse.OK
	}
}

func TestInterrupt(t *testing.T) {
	println("Testing interrupt")
	tmp, clean := setupInterruptTest(t)
	defer clean()

	time.Sleep(500 * time.Millisecond) // wait for filesystem to mount

	cmd := exec.Command("cat", filepath.Join(tmp, "/test"))
	cmd.Start()

	go func() {
		time.Sleep(500 * time.Millisecond) // wait for cat to start
		cmd.Process.Kill()
	}()

	t0 := time.Now()

	err := cmd.Wait()
	if err == nil {
		t.Fatalf("Error running command (it didn't return an error)")
	}
	println("Elapsed:", float32(time.Since(t0)) / float32(time.Second))
	if time.Since(t0) >= time.Duration(2 * time.Second) {
		t.Fatalf("Test took to much time")
	}
}
