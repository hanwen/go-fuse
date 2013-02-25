package fuse

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

var _ = log.Print

var xattrGolden = map[string][]byte{
	"user.attr1": []byte("val1"),
	"user.attr2": []byte("val2")}
var xattrFilename = "filename"

type XAttrTestFs struct {
	tester   *testing.T
	filename string
	attrs    map[string][]byte

	DefaultFileSystem
}

func NewXAttrFs(nm string, m map[string][]byte) *XAttrTestFs {
	x := new(XAttrTestFs)
	x.filename = nm
	x.attrs = make(map[string][]byte, len(m))
	for k, v := range m {
		x.attrs[k] = v
	}
	return x
}

func (fs *XAttrTestFs) GetAttr(name string, context *Context) (*Attr, Status) {
	a := &Attr{}
	if name == "" || name == "/" {
		a.Mode = S_IFDIR | 0700
		return a, OK
	}
	if name == fs.filename {
		a.Mode = S_IFREG | 0600
		return a, OK
	}
	return nil, ENOENT
}

func (fs *XAttrTestFs) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	fs.tester.Log("SetXAttr", name, attr, string(data), flags)
	if name != fs.filename {
		return ENOENT
	}
	dest := make([]byte, len(data))
	copy(dest, data)
	fs.attrs[attr] = dest
	return OK
}

func (fs *XAttrTestFs) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	if name != fs.filename {
		return nil, ENOENT
	}
	v, ok := fs.attrs[attr]
	if !ok {
		return nil, ENODATA
	}
	fs.tester.Log("GetXAttr", string(v))
	return v, OK
}

func (fs *XAttrTestFs) ListXAttr(name string, context *Context) (data []string, code Status) {
	if name != fs.filename {
		return nil, ENOENT
	}

	for k := range fs.attrs {
		data = append(data, k)
	}
	return data, OK
}

func (fs *XAttrTestFs) RemoveXAttr(name string, attr string, context *Context) Status {
	if name != fs.filename {
		return ENOENT
	}
	_, ok := fs.attrs[attr]
	fs.tester.Log("RemoveXAttr", name, attr, ok)
	if !ok {
		return ENODATA
	}
	delete(fs.attrs, attr)
	return OK
}

func readXAttr(p, a string) (val []byte, errno int) {
	val = make([]byte, 1024)
	return GetXAttr(p, a, val)
}

func xattrTestCase(t *testing.T, nm string) (mountPoint string, cleanup func()) {
	xfs := NewXAttrFs(nm, xattrGolden)
	xfs.tester = t
	mountPoint, err := ioutil.TempDir("", "go-fuse")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(mountPoint)

	nfs := NewPathNodeFs(xfs, nil)
	state, _, err := MountNodeFileSystem(mountPoint, nfs, nil)
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	state.Debug = VerboseTest()

	go state.Loop()
	return mountPoint, func() { state.Unmount() }
}

func TestXAttrNoExist(t *testing.T) {
	nm := xattrFilename
	mountPoint, clean := xattrTestCase(t, nm)
	defer clean()

	mounted := filepath.Join(mountPoint, nm)
	_, err := os.Lstat(mounted)
	if err != nil {
		t.Error("Unexpected stat error", err)
	}

	val, errno := readXAttr(mounted, "noexist")
	if errno == 0 {
		t.Error("Expected GetXAttr error", val)
	}
}

func TestXAttrRead(t *testing.T) {
	nm := xattrFilename
	mountPoint, clean := xattrTestCase(t, nm)
	defer clean()

	mounted := filepath.Join(mountPoint, nm)
	attrs, errno := ListXAttr(mounted)
	readback := make(map[string][]byte)
	if errno != 0 {
		t.Error("Unexpected ListXAttr error", errno)
	} else {
		for _, a := range attrs {
			val, errno := readXAttr(mounted, a)
			if errno != 0 {
				t.Errorf("GetXAttr(%q) failed: %v", a, syscall.Errno(errno))
			}
			readback[a] = val
		}
	}

	if len(readback) != len(xattrGolden) {
		t.Error("length mismatch", xattrGolden, readback)
	} else {
		for k, v := range readback {
			if bytes.Compare(xattrGolden[k], v) != 0 {
				t.Error("val mismatch", k, v, xattrGolden[k])
			}
		}
	}

	errno = Setxattr(mounted, "third", []byte("value"), 0)
	if errno != 0 {
		t.Error("Setxattr error", errno)
	}
	val, errno := readXAttr(mounted, "third")
	if errno != 0 || string(val) != "value" {
		t.Error("Read back set xattr:", errno, string(val))
	}

	Removexattr(mounted, "third")
	val, errno = readXAttr(mounted, "third")
	if errno != int(ENODATA) {
		t.Error("Data not removed?", errno, val)
	}
}
