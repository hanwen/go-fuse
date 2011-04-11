package examplelib

import (
	"github.com/hanwen/go-fuse/fuse"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var _ = strings.Join
var _ = log.Println

////////////////

// Create and mount filesystem.
const magicMode uint32 = 0753

type stackFsTestCase struct {
	origDir1 string
	origDir2 string
	mountDir string
	testDir  string
	tester   *testing.T

	fs    *SubmountFileSystem
	state *fuse.MountState
}

func (me *stackFsTestCase) Setup(t *testing.T) {
	me.tester = t

	me.testDir = fuse.MakeTempDir()
	me.origDir1 = filepath.Join(me.testDir, "orig1")
	me.origDir2 = filepath.Join(me.testDir, "orig2")
	me.mountDir = filepath.Join(me.testDir, "mount")

	os.Mkdir(me.origDir1, 0700)
	os.Mkdir(me.origDir2, 0700)
	os.Mkdir(me.mountDir, 0700)

	fs1 := fuse.NewPathFileSystemConnector(fuse.NewLoopbackFileSystem(me.origDir1))
	fs2 := fuse.NewPathFileSystemConnector(fuse.NewLoopbackFileSystem(me.origDir2))

	me.fs = NewSubmountFileSystem()

	attr := fuse.Attr{
		Mode: uint32(magicMode),
	}

	me.fs.AddFileSystem("sub1", fs1, attr)
	me.fs.AddFileSystem("sub2", fs2, attr)

	me.state = fuse.NewMountState(me.fs)
	me.state.Mount(me.mountDir)

	me.state.Debug = true

	fmt.Println("tempdir: ", me.testDir)

	// Unthreaded, but in background.
	go me.state.Loop(false)
}

// Unmount and del.
func (me *stackFsTestCase) Cleanup() {
	fmt.Println("Unmounting.")
	err := me.state.Unmount()
	CheckSuccess(err)
	os.RemoveAll(me.testDir)
}

////////////////

func (me *stackFsTestCase) testReaddir() {
	fmt.Println("testReaddir... ")
	dir, err := os.Open(me.mountDir)
	CheckSuccess(err)
	infos, err := dir.Readdir(10)
	CheckSuccess(err)

	wanted := map[string]bool{
		"sub1": true,
		"sub2": true,
	}
	if len(wanted) != len(infos) {
		me.tester.Errorf("Length mismatch %v", infos)
	} else {
		for _, v := range infos {
			_, ok := wanted[v.Name]
			if !ok {
				me.tester.Errorf("Unexpected name %v", v.Name)
			}

			if v.Mode&0777 != magicMode {
				me.tester.Errorf("Unexpected mode %o, %v", v.Mode, v)
			}
		}
	}

	dir.Close()
}


func (me *stackFsTestCase) testSubFs() {
	fmt.Println("testSubFs... ")
	for i := 1; i <= 2; i++ {
		// orig := filepath.Join(me.testDir, fmt.Sprintf("orig%d", i))
		mount := filepath.Join(me.mountDir, fmt.Sprintf("sub%d", i))

		name := "testFile"

		mountFile := filepath.Join(mount, name)

		f, err := os.OpenFile(mountFile, os.O_WRONLY, 0)
		if err == nil {
			me.tester.Errorf("Expected error for open write %v", name)
			continue
		}
		content1 := "booh!"
		f, err = os.Create(mountFile)
		CheckSuccess(err)

		f.Write([]byte(content1))
		f.Close()

		err = os.Chmod(mountFile, magicMode)
		CheckSuccess(err)

		fi, err := os.Lstat(mountFile)
		CheckSuccess(err)
		if fi.Mode&0777 != magicMode {
			me.tester.Errorf("Mode %o", fi.Mode)
		}

		g, err := os.Open(mountFile)
		CheckSuccess(err)

		buf := make([]byte, 1024)
		n, err := g.Read(buf)
		CheckSuccess(err)
		if string(buf[:n]) != content1 {
			me.tester.Errorf("content %v", buf[:n])
		}
		g.Close()
	}
}

func (me *stackFsTestCase) testAddRemove() {
	me.tester.Log("testAddRemove")
	attr := fuse.Attr{
		Mode: 0755,
	}

	conn := fuse.NewPathFileSystemConnector(fuse.NewLoopbackFileSystem(me.origDir1))
	ok := me.fs.AddFileSystem("sub1", conn, attr)
	if ok {
		me.tester.Errorf("AddFileSystem should fail")
		return
	}
	ok = me.fs.AddFileSystem("third", conn, attr)
	if !ok {
		me.tester.Errorf("AddFileSystem fail")
	}
	conn.Init(new(fuse.InHeader), new(fuse.InitIn))

	fi, err := os.Lstat(filepath.Join(me.mountDir, "third"))
	CheckSuccess(err)

	if !fi.IsDirectory() {
		me.tester.Errorf("not a directory %v", fi)
	}

	fs := me.fs.RemoveFileSystem("third")
	if fs == nil {
		me.tester.Errorf("remove fail")
	}
	dir, err := os.Open(me.mountDir)
	CheckSuccess(err)
	infos, err := dir.Readdir(10)
	CheckSuccess(err)
	if len(infos) != 2 {
		me.tester.Errorf("lstat expect 2 infos %v", infos)
	}
	dir.Close()

	_, err = os.Open(filepath.Join(me.mountDir, "third"))
	if err == nil {
		me.tester.Errorf("expect enoent %v", err)
	}
}

func TestStackFS(t *testing.T) {
	ts := new(stackFsTestCase)
	ts.Setup(t)

	ts.testReaddir()
	ts.testSubFs()
	ts.testAddRemove()

	ts.Cleanup()
}
