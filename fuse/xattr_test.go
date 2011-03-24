package fuse
import (
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

func TestXAttr(t *testing.T) {
	nm := "filename"
	xfs := NewXAttrFs(nm,
		map[string][]byte{
		"user.attr1": []byte("val1"),
		"user.attr2": []byte("val2")})

	connector := NewPathFileSystemConnector(xfs)
	mountPoint := MakeTempDir()

	state := NewMountState(connector)
	state.Mount(mountPoint)
	defer state.Unmount()

	go state.Loop(false)

	mounted := filepath.Join(mountPoint, nm)
	_, err := os.Lstat(mounted)
	if err != nil {
		t.Error("Unexpected stat error", err)
	}

	val, errno := GetXAttr(mounted, "noexist")
	if errno == 0 {
		t.Error("Expected GetXAttr error")
	}

	val, errno = GetXAttr(mounted, "user.attr1")
	if err != nil {
		t.Error("Unexpected GetXAttr error", errno)
	}

	if string(val) != "val1" {
		t.Error("Unexpected value", val)
	}
}

