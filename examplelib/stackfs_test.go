package examplelib

import (
	"github.com/hanwen/go-fuse/fuse"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"testing"
)

var _ = strings.Join
var _ = log.Println

////////////////

func IsDir(name string) bool {
	fi, _ := os.Lstat(name)
	return fi != nil && fi.IsDirectory()
}

func IsFile(name string) bool {
	fi, _ := os.Lstat(name)
	return fi != nil && fi.IsRegular()
}

func FileExists(name string) bool {
	_, err := os.Lstat(name)
	return err == nil
}

// Create and mount filesystem.
const magicMode uint32 = 0753

type testCase struct {
	origDir1      string
	origDir2      string
	mountDir      string
	testDir      string
	tester *testing.T

	fs           *SubmountFileSystem	
	state        *fuse.MountState
}

func (self *testCase) Setup(t *testing.T) {
	self.tester = t

	self.testDir = fuse.MakeTempDir()
	self.origDir1 = path.Join(self.testDir, "orig1")
	self.origDir2 = path.Join(self.testDir, "orig2")
	self.mountDir = path.Join(self.testDir, "mount")

	os.Mkdir(self.origDir1, 0700)
	os.Mkdir(self.origDir2, 0700)
	os.Mkdir(self.mountDir, 0700)
	
	fs1 := fuse.NewPathFileSystemConnector(NewPassThroughFuse(self.origDir1))
	fs2 := fuse.NewPathFileSystemConnector(NewPassThroughFuse(self.origDir2))

	self.fs = NewSubmountFileSystem()

	attr := fuse.Attr{
	Mode: uint32(magicMode),
	}

	self.fs.AddFileSystem("sub1", fs1, attr)
	self.fs.AddFileSystem("sub2", fs2, attr)
	
	self.state = fuse.NewMountState(self.fs)
	self.state.Mount(self.mountDir)

	self.state.Debug = true

	fmt.Println("tempdir: ", self.testDir)

	// Unthreaded, but in background.
	go self.state.Loop(false)
}

// Unmount and del.
func (self *testCase) Cleanup() {
	fmt.Println("Unmounting.")
	err := self.state.Unmount()
	if err != nil {
		self.tester.Errorf("Can't unmount a dir, err: %v", err)
	}
	os.RemoveAll(self.testDir)
}

////////////////

func (self *testCase) testReaddir() {
	fmt.Println("testReaddir... ")
	dir, err := os.Open(self.mountDir, os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("opendir err %v", err)
		return
	}
	infos, err := dir.Readdir(10)
	if err != nil {
		self.tester.Errorf("readdir err %v", err)
	}
	
	wanted := map[string]bool{
		"sub1": true,
		"sub2": true,
	}
	if len(wanted) != len(infos) {
		self.tester.Errorf("Length mismatch %v", infos)
	} else {
		for _, v := range infos {
			_, ok := wanted[v.Name]
			if !ok {
				self.tester.Errorf("Unexpected name %v", v.Name)
			}

			if v.Mode & 0777 != magicMode {
				self.tester.Errorf("Unexpected mode %o, %v", v.Mode, v)
			}
		}
	}

	dir.Close()
}


func (self *testCase) testSubFs() {
	fmt.Println("testSubFs... ")
	for i := 1; i <= 2; i++ {
		// orig := path.Join(self.testDir, fmt.Sprintf("orig%d", i))
		mount := path.Join(self.mountDir, fmt.Sprintf("sub%d", i))

		name := "testFile"
		
		mountFile := path.Join(mount, name)

		f, err := os.Open(mountFile, os.O_WRONLY, 0)
		if err == nil {
			self.tester.Errorf("Expected error for open write %v", name)
			continue
		}
		content1 := "booh!"
		f, err = os.Open(mountFile, os.O_WRONLY | os.O_CREATE, magicMode)
		if err != nil {
			self.tester.Errorf("Create %v", err)	
		}

		f.Write([]byte(content1))
		f.Close()

		err = os.Chmod(mountFile, magicMode)
		if err != nil {
			self.tester.Errorf("chmod %v", err)	
		}
		
		fi, err := os.Lstat(mountFile)
		if err != nil {
			self.tester.Errorf("Lstat %v", err)	
		} else {
			if fi.Mode & 0777 != magicMode {
				self.tester.Errorf("Mode %o", fi.Mode)	
			}
		}
		
		g, err := os.Open(mountFile, os.O_RDONLY, 0)
		if err != nil {
			self.tester.Errorf("Open %v", err)	
		} else {
			buf := make([]byte, 1024)
			n, err := g.Read(buf)
			if err != nil {
				self.tester.Errorf("read err %v", err)
			}
			if string(buf[:n]) != content1 {
				self.tester.Errorf("content %v", buf[:n])
			}
			g.Close()
		}
	}
}

func (self *testCase) testAddRemove() {
	self.tester.Log("testAddRemove")
	attr := fuse.Attr{
	Mode:0755,
	}

	conn := fuse.NewPathFileSystemConnector(NewPassThroughFuse(self.origDir1))
	ok := self.fs.AddFileSystem("sub1", conn, attr)
	if ok {
		self.tester.Errorf("AddFileSystem should fail")
		return
	}
	ok = self.fs.AddFileSystem("third", conn, attr)
	if !ok {
		self.tester.Errorf("AddFileSystem fail")
	}
	conn.Init(new(fuse.InHeader), new(fuse.InitIn)) 
	
	fi, err := os.Lstat(path.Join(self.mountDir, "third"))
	if err != nil {
		self.tester.Errorf("third lstat err %v", err)
	} else {
		if !fi.IsDirectory() {
			self.tester.Errorf("not a directory %v", fi)
		}
	}

	fs := self.fs.RemoveFileSystem("third")
	if fs == nil {
		self.tester.Errorf("remove fail")
	}
	dir, err := os.Open(self.mountDir, os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("opendir err %v", err)
		return
	}
	infos, err := dir.Readdir(10)
	if len(infos) != 2 {
		self.tester.Errorf("lstat expect 2 infos %v", infos)
	}
	dir.Close()

	_, err = os.Open(path.Join(self.mountDir, "third"), os.O_RDONLY, 0)
	if err == nil {
		self.tester.Errorf("expect enoent %v", err)
	}
}


// Test driver.
func TestMount(t *testing.T) {
	ts := new(testCase)
	ts.Setup(t)

	ts.testReaddir()
	ts.testSubFs()
	ts.testAddRemove()

	ts.Cleanup()
}
