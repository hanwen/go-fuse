package zipfs

/*

This provides a practical example of mounting Go-fuse path filesystems
on top of each other.

It is a file system that configures a Zip filesystem at /zipmount when
symlinking path/to/zipfile to /config/zipmount

*/

import (
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var _ = log.Printf

const (
	CONFIG_PREFIX = "config/"
)

////////////////////////////////////////////////////////////////

// MultiZipFs is a path filesystem that mounts zipfiles.  It needs a
// reference to the FileSystemConnector to be able to execute
// mounts.
type MultiZipFs struct {
	lock          sync.RWMutex
	zips          map[string]*MemTreeFs
	dirZipFileMap map[string]string

	nodeFs *fuse.PathNodeFs
	fuse.DefaultFileSystem
}

func NewMultiZipFs() *MultiZipFs {
	m := new(MultiZipFs)
	m.zips = make(map[string]*MemTreeFs)
	m.dirZipFileMap = make(map[string]string)
	return m
}

func (me *MultiZipFs) String() string {
	return "MultiZipFs"
}

func (me *MultiZipFs) OnMount(nodeFs *fuse.PathNodeFs) {
	me.nodeFs = nodeFs
}

func (me *MultiZipFs) OpenDir(name string, context *fuse.Context) (stream chan fuse.DirEntry, code fuse.Status) {
	me.lock.RLock()
	defer me.lock.RUnlock()

	stream = make(chan fuse.DirEntry, len(me.zips)+2)
	if name == "" {
		var d fuse.DirEntry
		d.Name = "config"
		d.Mode = fuse.S_IFDIR | 0700
		stream <- fuse.DirEntry(d)
	}

	if name == "config" {
		for k, _ := range me.zips {
			var d fuse.DirEntry
			d.Name = k
			d.Mode = fuse.S_IFLNK
			stream <- fuse.DirEntry(d)
		}
	}

	close(stream)
	return stream, fuse.OK
}

func (me *MultiZipFs) GetAttr(name string, context *fuse.Context) (*os.FileInfo, fuse.Status) {
	a := &os.FileInfo{}
	if name == "" {
		// Should not write in top dir.
		a.Mode = fuse.S_IFDIR | 0500
		return a, fuse.OK
	}

	if name == "config" {
		// TODO
		a.Mode = fuse.S_IFDIR | 0700
		return a, fuse.OK
	}

	dir, base := filepath.Split(name)
	if dir != "" && dir != CONFIG_PREFIX {
		return nil, fuse.ENOENT
	}
	submode := uint32(fuse.S_IFDIR | 0700)
	if dir == CONFIG_PREFIX {
		submode = fuse.S_IFLNK | 0600
	}

	me.lock.RLock()
	defer me.lock.RUnlock()

	a.Mode = submode
	_, hasDir := me.zips[base]
	if hasDir {
		return a, fuse.OK
	}

	return nil, fuse.ENOENT
}

func (me *MultiZipFs) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	dir, basename := filepath.Split(name)
	if dir == CONFIG_PREFIX {
		me.lock.Lock()
		defer me.lock.Unlock()

		zfs, ok := me.zips[basename]
		if ok {
			code = me.nodeFs.UnmountNode(zfs.Root().Inode())
			if !code.Ok() {
				return code
			}
			me.zips[basename] = nil, false
			me.dirZipFileMap[basename] = "", false
			return fuse.OK
		} else {
			return fuse.ENOENT
		}
	}
	return fuse.EPERM
}

func (me *MultiZipFs) Readlink(path string, context *fuse.Context) (val string, code fuse.Status) {
	dir, base := filepath.Split(path)
	if dir != CONFIG_PREFIX {
		return "", fuse.ENOENT
	}

	me.lock.Lock()
	defer me.lock.Unlock()

	zipfile, ok := me.dirZipFileMap[base]
	if !ok {
		return "", fuse.ENOENT
	}
	return zipfile, fuse.OK

}
func (me *MultiZipFs) Symlink(value string, linkName string, context *fuse.Context) (code fuse.Status) {
	dir, base := filepath.Split(linkName)
	if dir != CONFIG_PREFIX {
		return fuse.EPERM
	}

	me.lock.Lock()
	defer me.lock.Unlock()

	_, ok := me.dirZipFileMap[base]
	if ok {
		return fuse.EBUSY
	}

	fs, err := NewArchiveFileSystem(value)
	if err != nil {
		log.Println("NewZipArchiveFileSystem failed.", err)
		return fuse.EINVAL
	}

	code = me.nodeFs.Mount(base, fs, nil)
	if !code.Ok() {
		return code
	}

	me.dirZipFileMap[base] = value
	me.zips[base] = fs
	return fuse.OK
}
