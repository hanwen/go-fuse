package examplelib

import (
	"github.com/hanwen/go-fuse/fuse"
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"syscall"
	"rand"
	"time"
)

var _ = strings.Join
var _ = log.Println

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
	state        *fuse.MountState
	connector    *fuse.PathFileSystemConnector
}

// Create and mount filesystem.
func (self *testCase) Setup(t *testing.T) {
	self.tester = t

	const name string = "hello.txt"
	const subdir string = "subdir"

	self.origDir = fuse.MakeTempDir()
	self.mountPoint = fuse.MakeTempDir()

	self.mountFile = path.Join(self.mountPoint, name)
	self.mountSubdir = path.Join(self.mountPoint, subdir)
	self.mountSubfile = path.Join(self.mountSubdir, "subfile")
	self.origFile = path.Join(self.origDir, name)
	self.origSubdir = path.Join(self.origDir, subdir)
	self.origSubfile = path.Join(self.origSubdir, "subfile")
	
	pfs := NewPassThroughFuse(self.origDir)
	self.connector = fuse.NewPathFileSystemConnector(pfs)
	self.connector.Debug = true
	self.state = fuse.NewMountState(self.connector)
	self.state.Mount(self.mountPoint)

	//self.state.Debug = false
	self.state.Debug = true

	fmt.Println("Orig ", self.origDir, " mount ", self.mountPoint)

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
	self.tester.Log("Testing create.")
	f, err := os.Open(self.mountFile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		self.tester.Errorf("Fuse create/open %v", err)
	}

	self.tester.Log("Testing write.")
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
	self.tester.Log("Orig contents", slice[:n])
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
	self.tester.Log("Testing hard links.")
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
	self.tester.Log("testing symlink/readlink.")
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
	self.tester.Log("Testing rename.")
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
	self.tester.Log("Testing mknod.")
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
	self.tester.Log("Testing readdir.")
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

	wanted := map[string]bool{
		"hello.txt": true,
		"subdir":    true,
	}
	if len(wanted) != len(infos) {
		self.tester.Errorf("Length mismatch %v", infos)
	} else {
		for _, v := range infos {
			_, ok := wanted[v.Name]
			if !ok {
				self.tester.Errorf("Unexpected name %v", v.Name)
			}
		}
	}

	dir.Close()

	self.removeMountSubdir()
	self.removeMountFile()
}

func (self *testCase) testFSync() {
	self.tester.Log("Testing fsync.")
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

func (self *testCase) testLargeRead() {
	self.tester.Log("Testing large read.")
	name := path.Join(self.origDir, "large")
	f, err := os.Open(name, os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		self.tester.Errorf("open write err %v", err)
	}

	b := bytes.NewBuffer(nil)

	for i := 0; i < 20*1024; i++ {
		b.WriteString("bla")
	}
	b.WriteString("something extra to not be round")

	slice := b.Bytes()
	n, err := f.Write(slice)
	if err != nil {
		self.tester.Errorf("write err %v %v", err, n)
	}

	err = f.Close()
	if err != nil {
		self.tester.Errorf("close err %v", err)
	}

	// Read in one go.
	g, err := os.Open(path.Join(self.mountPoint, "large"), os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("open err %v", err)
	}
	readSlice := make([]byte, len(slice))
	m, err := g.Read(readSlice)
	if m != n {
		self.tester.Errorf("read mismatch %v %v", m, n)
	}
	for i, v := range readSlice {
		if slice[i] != v {
			self.tester.Errorf("char mismatch %v %v %v", i, slice[i], v)
			break
		}
	}

	if err != nil {
		self.tester.Errorf("read mismatch %v", err)
	}
	g.Close()

	// Read in chunks
	g, err = os.Open(path.Join(self.mountPoint, "large"), os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("open err %v", err)
	}
	readSlice = make([]byte, 4096)
	total := 0
	for {
		m, err := g.Read(readSlice)
		if m == 0 && err == os.EOF {
			break
		}
		if err != nil {
			self.tester.Errorf("read err %v %v", err, m)
			break
		}
		total += m
	}
	if total != len(slice) {
		self.tester.Errorf("slice error %d", total)
	}
	g.Close()

	os.Remove(name)
}

func randomLengthString(length int) string {
	r := rand.Intn(length)
	j := 0

	b := make([]byte, r)
	for i := 0; i < r; i++ {
		j = (j + 1) % 10
		b[i] = byte(j) + byte('0')
	}
	return string(b)
}


func (self *testCase) testLargeDirRead() {
	self.tester.Log("Testing large readdir.")
	created := 100

	names := make([]string, created)

	subdir := path.Join(self.origDir, "readdirSubdir")
	os.Mkdir(subdir, 0700)
	longname := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	nameSet := make(map[string]bool)
	for i := 0; i < created; i++ {
		// Should vary file name length.
		base := fmt.Sprintf("file%d%s", i,
			randomLengthString(len(longname)))
		name := path.Join(subdir, base)

		nameSet[base] = true

		f, err := os.Open(name, os.O_WRONLY|os.O_CREATE, 0777)
		if err != nil {
			self.tester.Errorf("open write err %v", err)
			break
		}
		f.WriteString("bla")
		f.Close()

		names[i] = name
	}

	dir, err := os.Open(path.Join(self.mountPoint, "readdirSubdir"), os.O_RDONLY, 0)
	if err != nil {
		self.tester.Errorf("dirread %v", err)
	}
	// Chunked read.
	total := 0
	readSet := make(map[string]bool)
	for {
		namesRead, err := dir.Readdirnames(200)
		if err != nil {
			self.tester.Errorf("readdir err %v %v", err, namesRead)
		}

		if len(namesRead) == 0 {
			break
		}
		for _, v := range namesRead {
			readSet[v] = true
		}
		total += len(namesRead)
	}

	if total != created {
		self.tester.Errorf("readdir mismatch got %v wanted %v", total, created)
	}
	for k, _ := range nameSet {
		_, ok := readSet[k]
		if !ok {
			self.tester.Errorf("Name %v not found in output", k)
		}
	}

	dir.Close()

	os.RemoveAll(subdir)
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
	ts.testLargeRead()
	ts.testLargeDirRead()
	ts.Cleanup()
}

func TestRecursiveMount(t *testing.T) {
	ts := new(testCase)
	ts.Setup(t)

	f, err := os.Open(path.Join(ts.mountPoint, "hello.txt"),
		os.O_WRONLY|os.O_CREATE, 0777)
	
	if err != nil {
		t.Errorf("open write err %v", err)
	}
	f.WriteString("bla")
	f.Close()
	
	pfs2 := NewPassThroughFuse(ts.origDir)
	code := ts.connector.Mount("/hello.txt", pfs2)
	if code != fuse.EINVAL {
		t.Error("expect EINVAL", code)
	}

	submnt := path.Join(ts.mountPoint, "mnt")
	err = os.Mkdir(submnt, 0777)
	if err != nil {
		t.Errorf("mkdir")
	}
	
	code = ts.connector.Mount("/mnt", pfs2)
	if code != fuse.OK {
		t.Errorf("mkdir")
	}
	
	_, err = os.Lstat(submnt)
	if err != nil {
		t.Error("lstat submount", err)
	}
	_, err = os.Lstat(path.Join(submnt, "hello.txt"))
	if err != nil {
		t.Error("lstat submount/file", err)
	}
	
	f, err = os.Open(path.Join(submnt, "hello.txt"), os.O_RDONLY, 0)
	if err != nil {
		t.Error("open submount/file", err)
	}
	code = ts.connector.Unmount("/mnt")
	if code != fuse.EBUSY {
		t.Error("expect EBUSY")
	}

	f.Close()

	// The close takes some time to propagate through FUSE.
	time.Sleep(1e9)
	
	code = ts.connector.Unmount("/mnt")
	if code != fuse.OK {
		t.Error("umount failed.", code)
	}

	ts.Cleanup()
}
