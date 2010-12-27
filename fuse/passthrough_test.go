package fuse

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"syscall"
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

////////////////
// state for our testcase, mostly constants

const contents string = "ABC"
const mode uint32 = 0757

type testCase struct {
	origDir      string
	mountPoint   string
	mountFile    string
	mountSubdir  string
	mountSubfile string
	origFile     string
	origSubdir   string
	origSubfile  string
	tester       *testing.T
	state        *MountState
}

// Create and mount filesystem.
func (self *testCase) Setup(t *testing.T) {
	self.tester = t

	const name string = "hello.txt"
	const subdir string = "subdir"

	self.origDir = MakeTempDir()
	self.mountPoint = MakeTempDir()

	self.mountFile = path.Join(self.mountPoint, name)
	self.mountSubdir = path.Join(self.mountPoint, subdir)
	self.mountSubfile = path.Join(self.mountSubdir, "subfile")
	self.origFile = path.Join(self.origDir, name)
	self.origSubdir = path.Join(self.origDir, subdir)
	self.origSubfile = path.Join(self.origSubdir, "subfile")

	fs := NewPathFileSystemConnector(NewPassThroughFuse(self.origDir))

	self.state = NewMountState(fs)
	self.state.Mount(self.mountPoint, false)

	//self.state.Debug = false
	self.state.Debug = true

	fmt.Println("Orig ", self.origDir, " mount ", self.mountPoint)
}

// Unmount and del.
func (self *testCase) Cleanup() {
	fmt.Println("Unmounting.")
	err := self.state.Unmount()
	if err != nil {
		self.tester.Errorf("Can't unmount a dir, err: %v", err)
	}
	os.Remove(self.mountPoint)
	os.RemoveAll(self.origDir)
}

////////////////
// Utilities.

func (self *testCase) makeOrigSubdir() {
	err := os.Mkdir(self.origSubdir, 0777)
	if err != nil {
		self.tester.Errorf("orig mkdir subdir %v", err)
	}
}


func (self *testCase) removeMountSubdir() {
	err := os.RemoveAll(self.mountSubdir)
	if err != nil {
		self.tester.Errorf("orig rmdir subdir %v", err)
	}
}

func (self *testCase) removeMountFile() {
	os.Remove(self.mountFile)
	// ignore errors.
}

func (self *testCase) writeOrigFile() {
	f, err := os.Open(self.origFile, os.O_WRONLY|os.O_CREAT, 0700)
	if err != nil {
		self.tester.Errorf("Error orig open: %v", err)
	}
	_, err = f.Write([]byte(contents))
	if err != nil {
		self.tester.Errorf("Write %v", err)
	}
	f.Close()
}

////////////////
// Tests.

func (self *testCase) testOpenUnreadable() {
	_, err := os.Open(path.Join(self.mountPoint, "doesnotexist"), os.O_RDONLY, 0)
	if err == nil {
		self.tester.Errorf("open non-existent should raise error")
	}
}

func (self *testCase) testReadThroughFuse() {
	self.writeOrigFile()

	fmt.Println("Testing chmod.")
	err := os.Chmod(self.mountFile, mode)
	if err != nil {
		self.tester.Errorf("Chmod %v", err)
	}

	fmt.Println("Testing Lstat.")
	fi, err := os.Lstat(self.mountFile)
	if err != nil {
		self.tester.Errorf("Lstat %v", err)
	}
	if (fi.Mode & 0777) != mode {
		self.tester.Errorf("Wrong mode %o != %o", fi.Mode, mode)
	}

	// Open (for read), read. 
	fmt.Println("Testing open.")
	f, err := os.Open(self.mountFile, os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("Fuse open %v", err)
	}

	fmt.Println("Testing read.")
	var buf [1024]byte
	slice := buf[:]
	n, err := f.Read(slice)

	if len(slice[:n]) != len(contents) {
		self.tester.Errorf("Content error %v", slice)
	}
	fmt.Println("Testing close.")
	f.Close()

	self.removeMountFile()
}

func (self *testCase) testRemove() {
	self.writeOrigFile()

	fmt.Println("Testing remove.")
	err := os.Remove(self.mountFile)
	if err != nil {
		self.tester.Errorf("Remove %v", err)
	}
	_, err = os.Lstat(self.origFile)
	if err == nil {
		self.tester.Errorf("Lstat() after delete should have generated error.")
	}
}

func (self *testCase) testWriteThroughFuse() {
	// Create (for write), write. 
	fmt.Println("Testing create.")
	f, err := os.Open(self.mountFile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		self.tester.Errorf("Fuse create/open %v", err)
	}

	fmt.Println("Testing write.")
	n, err := f.WriteString(contents)
	if err != nil {
		self.tester.Errorf("fuse write %v", err)
	}
	if n != len(contents) {
		self.tester.Errorf("Write mismatch: %v of %v", n, len(contents))
	}

	fi, err := os.Lstat(self.origFile)
	if fi.Mode&0777 != 0644 {
		self.tester.Errorf("create mode error %o", fi.Mode&0777)
	}

	f, err = os.Open(self.origFile, os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("orig open %v", err)
	}
	var buf [1024]byte
	slice := buf[:]
	n, err = f.Read(slice)
	if err != nil {
		self.tester.Errorf("orig read %v", err)
	}
	fmt.Println("Orig contents", slice[:n])
	if string(slice[:n]) != contents {
		self.tester.Errorf("write contents error %v", slice[:n])
	}
	f.Close()
	self.removeMountFile()
}

func (self *testCase) testMkdirRmdir() {
	// Mkdir/Rmdir.
	err := os.Mkdir(self.mountSubdir, 0777)
	if err != nil {
		self.tester.Errorf("mount mkdir", err)
	}
	fi, err := os.Lstat(self.origSubdir)
	if !fi.IsDirectory() {
		self.tester.Errorf("Not a directory: %o", fi.Mode)
	}

	err = os.Remove(self.mountSubdir)
	if err != nil {
		self.tester.Errorf("rmdir %v", err)
	}
}

func (self *testCase) testLink() {
	fmt.Println("Testing hard links.")
	self.writeOrigFile()
	err := os.Mkdir(self.origSubdir, 0777)
	if err != nil {
		self.tester.Errorf("mount mkdir", err)
	}

	// Link.
	err = os.Link(self.mountFile, self.mountSubfile)
	if err != nil {
		self.tester.Errorf("mount link %v", err)
	}

	fi, err := os.Lstat(self.mountFile)
	if fi.Nlink != 2 {
		self.tester.Errorf("Expect 2 links: %v", fi)
	}

	f, err := os.Open(self.mountSubfile, os.O_RDONLY, 0)

	var buf [1024]byte
	slice := buf[:]
	n, err := f.Read(slice)
	f.Close()

	strContents := string(slice[:n])
	if strContents != contents {
		self.tester.Errorf("Content error: %v", slice[:n])
	}
	self.removeMountSubdir()
	self.removeMountFile()
}

func (self *testCase) testSymlink() {
	fmt.Println("testing symlink/readlink.")
	self.writeOrigFile()

	linkFile := "symlink-file"
	orig := "hello.txt"
	err := os.Symlink(orig, path.Join(self.mountPoint, linkFile))
	defer os.Remove(path.Join(self.mountPoint, linkFile))
	defer self.removeMountFile()

	if err != nil {
		self.tester.Errorf("symlink %v", err)
	}

	origLink := path.Join(self.origDir, linkFile)
	fi, err := os.Lstat(origLink)
	if err != nil {
		self.tester.Errorf("link lstat %v", err)
		return
	}

	if !fi.IsSymlink() {
		self.tester.Errorf("not a symlink: %o", fi.Mode)
		return
	}

	read, err := os.Readlink(path.Join(self.mountPoint, linkFile))
	if err != nil {
		self.tester.Errorf("orig readlink %v", err)
		return
	}

	if read != orig {
		self.tester.Errorf("unexpected symlink value '%v'", read)
	}
}

func (self *testCase) testRename() {
	fmt.Println("Testing rename.")
	self.writeOrigFile()
	self.makeOrigSubdir()

	err := os.Rename(self.mountFile, self.mountSubfile)
	if err != nil {
		self.tester.Errorf("rename %v", err)
	}
	if FileExists(self.origFile) {
		self.tester.Errorf("original %v still exists.", self.origFile)
	}
	if !FileExists(self.origSubfile) {
		self.tester.Errorf("destination %v does not exist.", self.origSubfile)
	}

	self.removeMountSubdir()
}


func (self *testCase) testAccess() {
	self.writeOrigFile()
	err := os.Chmod(self.origFile, 0)
	if err != nil {
		self.tester.Errorf("chmod %v", err)
	}

	// Ugh - copied from unistd.h
	const W_OK uint32 = 2

	errCode := syscall.Access(self.mountFile, W_OK)
	if errCode != syscall.EACCES {
		self.tester.Errorf("Expected EACCES for non-writable, %v %v", errCode, syscall.EACCES)
	}
	err = os.Chmod(self.origFile, 0222)
	if err != nil {
		self.tester.Errorf("chmod %v", err)
	}

	errCode = syscall.Access(self.mountFile, W_OK)
	if errCode != 0 {
		self.tester.Errorf("Expected no error code for writable. %v", errCode)
	}
	self.removeMountFile()
	self.removeMountFile()
}

func (self *testCase) testMknod() {
	fmt.Println("Testing mknod.")
	errNo := syscall.Mknod(self.mountFile, syscall.S_IFIFO|0777, 0)
	if errNo != 0 {
		self.tester.Errorf("Mknod %v", errNo)
	}
	fi, _ := os.Lstat(self.origFile)
	if fi == nil || !fi.IsFifo() {
		self.tester.Errorf("Expected FIFO filetype.")
	}

	self.removeMountFile()
}

func (self *testCase) testReaddir() {
	fmt.Println("Testing rename.")
	self.writeOrigFile()
	self.makeOrigSubdir()

	dir, err := os.Open(self.mountPoint, os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("opendir err %v", err)
		return
	}
	infos, err := dir.Readdir(10)
	if err != nil {
		self.tester.Errorf("readdir err %v", err)
	}

	if len(infos) != 2 {
		self.tester.Errorf("infos mismatch %v", infos)
	} else {
		if infos[0].Name != "hello.txt" || infos[1].Name != "subdir" {
			self.tester.Errorf("names incorrect %v", infos)
		}
	}

	dir.Close()

	self.removeMountSubdir()
	self.removeMountFile()
}

func (self *testCase) testFSync() {
	fmt.Println("Testing rename.")
	self.writeOrigFile()

	f, err := os.Open(self.mountFile, os.O_WRONLY, 0)
	_, err = f.WriteString("hello there")
	if err != nil {
		self.tester.Errorf("writestring %v", err)
	}

	// How to really test fsync ?
	errNo := syscall.Fsync(f.Fd())
	if errNo != 0 {
		self.tester.Errorf("fsync returned %v", errNo)
	}
	f.Close()
}


// Test driver.
func TestMount(t *testing.T) {
	ts := new(testCase)
	ts.Setup(t)

	ts.testOpenUnreadable()
	ts.testReadThroughFuse()
	ts.testRemove()
	ts.testMkdirRmdir()
	ts.testLink()
	ts.testSymlink()
	ts.testRename()
	ts.testAccess()
	ts.testMknod()
	ts.testReaddir()
	ts.testFSync()

	ts.Cleanup()
}
