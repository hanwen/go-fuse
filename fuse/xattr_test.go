package fuse

import (
	"bytes"
	"io/ioutil"
	"testing"
	"log"
	"path/filepath"
	"os"
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

func (me *XAttrTestFs) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	a := &os.FileInfo{}
	if name == "" || name == "/" {
		a.Mode = S_IFDIR | 0700
		return a, OK
	}
	if name == me.filename {
		a.Mode = S_IFREG | 0600
		return a, OK
	}
	return nil, ENOENT
}

func (me *XAttrTestFs) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	me.tester.Log("SetXAttr", name, attr, string(data), flags)
	if name != me.filename {
		return ENOENT
	}
	dest := make([]byte, len(data))
	copy(dest, data)
	me.attrs[attr] = dest
	return OK
}

func (me *XAttrTestFs) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	if name != me.filename {
		return nil, ENOENT
	}
	v, ok := me.attrs[attr]
	if !ok {
		return nil, ENODATA
	}
	me.tester.Log("GetXAttr", string(v))
	return v, OK
}

func (me *XAttrTestFs) ListXAttr(name string, context *Context) (data []string, code Status) {
	if name != me.filename {
		return nil, ENOENT
	}

	for k, _ := range me.attrs {
		data = append(data, k)
	}
	return data, OK
}

func (me *XAttrTestFs) RemoveXAttr(name string, attr string, context *Context) Status {
	if name != me.filename {
		return ENOENT
	}
	_, ok := me.attrs[attr]
	me.tester.Log("RemoveXAttr", name, attr, ok)
	if !ok {
		return ENODATA
	}
	me.attrs[attr] = nil, false
	return OK
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

	state, _, err := MountPathFileSystem(mountPoint, xfs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()
	defer state.Unmount()

	go state.Loop()

	mounted := filepath.Join(mountPoint, nm)
	_, err = os.Lstat(mounted)
	if err != nil {
		t.Error("Unexpected stat error", err)
	}

	val, errno := GetXAttr(mounted, "noexist")
	if errno == 0 {
		t.Error("Expected GetXAttr error", val)
	}

	attrs, errno := ListXAttr(mounted)

	readback := make(map[string][]byte)
	if errno != 0 {
		t.Error("Unexpected ListXAttr error", errno)
	} else {
		for _, a := range attrs {
			val, errno = GetXAttr(mounted, a)
			if errno != 0 {
				t.Error("Unexpected GetXAttr error", errno)
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
	val, errno = GetXAttr(mounted, "third")
	if errno != 0 || string(val) != "value" {
		t.Error("Read back set xattr:", errno, string(val))
	}

	Removexattr(mounted, "third")
	val, errno = GetXAttr(mounted, "third")
	if errno != int(ENODATA) {
		t.Error("Data not removed?", errno, val)
	}
}
