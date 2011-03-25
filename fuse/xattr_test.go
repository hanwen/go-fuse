package fuse

import (
	"bytes"
	"testing"
	"path/filepath"
	"os"
	"syscall"
)

type XAttrTestFs struct {
	filename string
	attrs map[string][]byte

	DefaultPathFilesystem
}

func NewXAttrFs(nm string, m map[string][]byte) *XAttrTestFs {
	x := new(XAttrTestFs)
	x.filename = nm
	x.attrs = m
	return x
}

func (me *XAttrTestFs) GetAttr(name string) (*Attr, Status) {
	a := new(Attr)
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

func (me *XAttrTestFs) SetXAttr(name string, attr string, data []byte, flags int) (Status) {
	if name != me.filename {
		return ENOENT
	}
	me.attrs[attr] = data
	return OK
}

func (me *XAttrTestFs) GetXAttr(name string, attr string) ([]byte, Status) {
	if name != me.filename {
		return nil, ENOENT
	}
	v, ok := me.attrs[attr]
	if !ok {
		return nil, syscall.ENODATA
	}

	return v, OK
}

func (me *XAttrTestFs) ListXAttr(name string) (data []string, code Status) {
	if name != me.filename {
		return nil, ENOENT
	}

	for k, _ := range me.attrs {
		data = append(data, k)
	}
	return data, OK
}

func TestXAttrRead(t *testing.T) {
	nm := "filename"

	golden := map[string][]byte{
		"user.attr1": []byte("val1"),
		"user.attr2": []byte("val2")}
	xfs := NewXAttrFs(nm, golden)

	connector := NewPathFileSystemConnector(xfs)
	mountPoint := MakeTempDir()

	state := NewMountState(connector)
	state.Mount(mountPoint)
	state.Debug = true
	defer state.Unmount()

	go state.Loop(false)

	mounted := filepath.Join(mountPoint, nm)
	_, err := os.Lstat(mounted)
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
		for k, v := range(readback) {
			if bytes.Compare(golden[k], v) != 0 {
				t.Error("val mismatch", k, v, golden[k])
			}
		}
	}

	Setxattr(mounted, "third", []byte("value"), 0)
	val, errno = GetXAttr(mounted, "third")
	if errno != 0 || string(val) != "value" {
		t.Error("Read back set xattr:", err, val)
	}
}

