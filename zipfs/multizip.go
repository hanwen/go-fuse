package zipfs

/*

This provides a practical example of mounting Go-fuse path filesystems
on top of each other.

It is a file system that configures a Zip filesystem at /zipmount when writing
path/to/zipfile to /config/zipmount

*/

import (
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"path/filepath"
	"sync"
	"strings"
)

var _ = log.Printf

const (
	CONFIG_PREFIX = "config/"
)

// zipCreateFile is a placeholder file to receive the write containing
// the path to the zip file.
type zipCreateFile struct {
	// Basename of the entry in the FS.
	Basename string
	zfs      *MultiZipFs

	fuse.DefaultFile
}

func (me *zipCreateFile) Write(input *fuse.WriteIn, nameBytes []byte) (uint32, fuse.Status) {
	if me.zfs == nil {
		// TODO
		return 0, fuse.EPERM
	}
	zipFile := string(nameBytes)

	zipFile = strings.Trim(zipFile, "\n ")
	fs := NewZipArchiveFileSystem(zipFile)
	if fs == nil {
		// TODO
		log.Println("NewZipArchiveFileSystem returned nil")
		me.zfs.pendingZips[me.Basename] = false, false
		return 0, fuse.ENOSYS
	}

	code := me.zfs.Connector.Mount("/"+filepath.Base(me.Basename), fs)
	if code != fuse.OK {
		return 0, code

	}
	// TODO. locks?

	me.zfs.zips[me.Basename] = fs
	me.zfs.pendingZips[me.Basename] = false, false

	me.zfs = nil

	return uint32(len(nameBytes)), code
}

////////////////////////////////////////////////////////////////

// MultiZipFs is a path filesystem that mounts zipfiles.  It needs a
// reference to the PathFileSystemConnector to be able to execute
// mounts.
type MultiZipFs struct {
	Connector    *fuse.PathFileSystemConnector
	lock         sync.RWMutex
	zips         map[string]*ZipArchiveFileSystem
	pendingZips  map[string]bool
	zipFileNames map[string]string

	fuse.DefaultPathFileSystem
}

func NewMultiZipFs() *MultiZipFs {
	m := new(MultiZipFs)
	m.zips = make(map[string]*ZipArchiveFileSystem)
	m.pendingZips = make(map[string]bool)
	m.zipFileNames = make(map[string]string)
	m.Connector = fuse.NewPathFileSystemConnector(m)
	return m
}

func (me *MultiZipFs) OpenDir(name string) (stream chan fuse.DirEntry, code fuse.Status) {
	me.lock.RLock()
	defer me.lock.RUnlock()

	// We don't use a goroutine, since we don't want to hold the
	// lock.
	stream = make(chan fuse.DirEntry,
		len(me.pendingZips)+len(me.zips)+2)

	submode := uint32(fuse.S_IFDIR | 0700)
	if name == "config" {
		submode = fuse.S_IFREG | 0600
	}

	for k, _ := range me.zips {
		var d fuse.DirEntry
		d.Name = k
		d.Mode = submode
		stream <- fuse.DirEntry(d)
	}
	for k, _ := range me.pendingZips {
		var d fuse.DirEntry
		d.Name = k
		d.Mode = submode
		stream <- fuse.DirEntry(d)
	}

	if name == "" {
		var d fuse.DirEntry
		d.Name = "config"
		d.Mode = fuse.S_IFDIR | 0700
		stream <- fuse.DirEntry(d)
	}

	close(stream)
	return stream, fuse.OK
}

func (me *MultiZipFs) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	a := new(fuse.Attr)
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
		submode = fuse.S_IFREG | 0600
	}

	me.lock.RLock()
	defer me.lock.RUnlock()

	a.Mode = submode
	entry, hasDir := me.zips[base]
	if hasDir {
		a.Size = uint64(len(entry.ZipFileName))
		return a, fuse.OK
	}
	_, hasDir = me.pendingZips[base]
	if hasDir {
		return a, fuse.OK
	}

	return nil, fuse.ENOENT
}

func (me *MultiZipFs) Unlink(name string) (code fuse.Status) {
	dir, basename := filepath.Split(name)
	if dir == CONFIG_PREFIX {
		me.lock.Lock()
		defer me.lock.Unlock()

		_, ok := me.zips[basename]
		if ok {
			me.zips[basename] = nil, false
			return fuse.OK
		} else {
			return fuse.ENOENT
		}
	}
	return fuse.EPERM
}

func (me *MultiZipFs) Open(name string, flags uint32) (file fuse.File, code fuse.Status) {
	if 0 != flags&uint32(fuse.O_ANYWRITE) {
		return nil, fuse.EPERM
	}

	dir, basename := filepath.Split(name)
	if dir == CONFIG_PREFIX {
		me.lock.RLock()
		defer me.lock.RUnlock()

		entry, ok := me.zips[basename]
		if !ok {
			return nil, fuse.ENOENT
		}

		return fuse.NewReadOnlyFile([]byte(entry.ZipFileName)), fuse.OK
	}

	return nil, fuse.ENOENT
}

func (me *MultiZipFs) Create(name string, flags uint32, mode uint32) (file fuse.File, code fuse.Status) {
	dir, base := filepath.Split(name)
	if dir != CONFIG_PREFIX {
		return nil, fuse.EPERM
	}

	z := new(zipCreateFile)
	z.Basename = base
	z.zfs = me

	me.lock.Lock()
	defer me.lock.Unlock()

	me.pendingZips[z.Basename] = true

	return z, fuse.OK
}
