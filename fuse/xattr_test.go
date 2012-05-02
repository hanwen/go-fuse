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

type XAttrTestFs struct {
	tester   *testing.T
	filename string
	attrs    map[string][]byte

	DefaultFileSystem
}

func NewXAttrFs(nm string, m map[string][]byte) *XAttrTestFs {
	x := new(XAttrTestFs)
	x.filename = nm
	x.attrs = m
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

func TestXAttrRead(t *testing.T) {
	nm := "filename"

	golden := map[string][]byte{
		"user.attr1": []byte("val1"),
		"user.attr2": []byte("val2")}
	xfs := NewXAttrFs(nm, golden)
	xfs.tester = t
	mountPoint, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	defer os.RemoveAll(mountPoint)

	nfs := NewPathNodeFs(xfs, nil)
	state, _, err := MountNodeFileSystem(mountPoint, nfs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()
	defer state.Unmount()

	go state.Loop()

	mounted := filepath.Join(mountPoint, nm)
	_, err = os.Lstat(mounted)
	if err != nil {
		t.Error("Unexpected stat error", err)
	}

	val, errno := readXAttr(mounted, "noexist")
	if errno == 0 {
		t.Error("Expected GetXAttr error", val)
	}

	attrs, errno := ListXAttr(mounted)
	readback := make(map[string][]byte)
	if errno != 0 {
		t.Error("Unexpected ListXAttr error", errno)
	} else {
		for _, a := range attrs {
			val, errno = readXAttr(mounted, a)
			if errno != 0 {
				t.Error("Unexpected GetXAttr error", syscall.Errno(errno))
			}
			readback[a] = val
		}
	}

	if len(readback) != len(golden) {
		t.Error("length mismatch", golden, readback)
	} else {
		for k, v := range readback {
			if bytes.Compare(golden[k], v) != 0 {
				t.Error("val mismatch", k, v, golden[k])
			}
		}
	}

	errno = Setxattr(mounted, "third", []byte("value"), 0)
	if errno != 0 {
		t.Error("Setxattr error", errno)
	}
	val, errno = readXAttr(mounted, "third")
	if errno != 0 || string(val) != "value" {
		t.Error("Read back set xattr:", errno, string(val))
	}

	Removexattr(mounted, "third")
	val, errno = readXAttr(mounted, "third")
	if errno != int(ENODATA) {
		t.Error("Data not removed?", errno, val)
	}
}
