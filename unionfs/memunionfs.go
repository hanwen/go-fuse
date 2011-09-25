package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var _ = log.Println

// A unionfs that only uses on-disk backing store for file contents.
type MemUnionFs struct {
	fuse.DefaultNodeFileSystem
	backingStore string
	root         *memNode

	mutex    sync.Mutex
	nextFree int

	readonly fuse.FileSystem
}

type memNode struct {
	fuse.DefaultFsNode
	fs *MemUnionFs

	original string

	// protects mutable data below.
	mutex   sync.RWMutex
	backing string
	changed bool
	link    string
	info    os.FileInfo
	deleted map[string]bool
}

func (me *MemUnionFs) getFilename() string {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	id := me.nextFree
	me.nextFree++
	return fmt.Sprintf("%s/%d", me.backingStore, id)
}

func (me *MemUnionFs) Root() fuse.FsNode {
	return me.root
}

func (me *MemUnionFs) StatFs() *fuse.StatfsOut {
	backingFs := &fuse.LoopbackFileSystem{Root: me.backingStore}
	return backingFs.StatFs()
}

func (me *MemUnionFs) newNode(isdir bool) *memNode {
	n := &memNode{
		fs: me,
	}
	if isdir {
		n.deleted = map[string]bool{}
	}
	now := time.Nanoseconds()
	n.info.Mtime_ns = now
	n.info.Atime_ns = now
	n.info.Ctime_ns = now
	return n
}

func NewMemUnionFs(backingStore string, roFs fuse.FileSystem) *MemUnionFs {
	me := &MemUnionFs{}
	me.backingStore = backingStore
	me.readonly = roFs
	me.root = me.newNode(true)
	return me
}

func (me *memNode) Deletable() bool {
	return !me.changed
}

func (me *memNode) touch() {
	me.changed = true
	me.info.Mtime_ns = time.Nanoseconds()
}

func (me *memNode) ctouch() {
	me.changed = true
	me.info.Ctime_ns = time.Nanoseconds()
}

func (me *memNode) newNode(isdir bool) *memNode {
	n := me.fs.newNode(isdir)
	me.Inode().New(isdir, n)
	return n
}

func (me *memNode) Readlink(c *fuse.Context) ([]byte, fuse.Status) {
	me.mutex.RLock()
	defer me.mutex.RUnlock()
	return []byte(me.link), fuse.OK
}

func (me *memNode) Lookup(name string, context *fuse.Context) (fi *os.FileInfo, node fuse.FsNode, code fuse.Status) {
	me.mutex.RLock()
	defer me.mutex.RUnlock()

	if _, del := me.deleted[name]; del {
		return nil, nil, fuse.ENOENT
	}

	if me.original == "" && me != me.fs.root {
		return nil, nil, fuse.ENOENT
	}

	fn := filepath.Join(me.original, name)
	fi, code = me.fs.readonly.GetAttr(fn, context)
	if !code.Ok() {
		return nil, nil, code
	}

	child := me.newNode(fi.Mode&fuse.S_IFDIR != 0)
	child.info = *fi
	child.original = fn
	if child.info.Mode&fuse.S_IFLNK != 0 {
		child.link, _ = me.fs.readonly.Readlink(fn, context)
	}
	me.Inode().AddChild(name, child.Inode())

	return fi, child, fuse.OK
}

func (me *memNode) Mkdir(name string, mode uint32, context *fuse.Context) (fi *os.FileInfo, newNode fuse.FsNode, code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()

	me.deleted[name] = false, false
	n := me.newNode(true)
	n.changed = true
	n.info.Mode = mode | fuse.S_IFDIR
	me.Inode().AddChild(name, n.Inode())
	me.touch()
	return &n.info, n, fuse.OK
}

func (me *memNode) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()

	me.deleted[name] = true
	ch := me.Inode().RmChild(name)
	if ch == nil {
		return fuse.ENOENT
	}
	me.touch()

	return fuse.OK
}

func (me *memNode) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	return me.Unlink(name, context)
}

func (me *memNode) Symlink(name string, content string, context *fuse.Context) (fi *os.FileInfo, newNode fuse.FsNode, code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	n := me.newNode(false)
	n.info.Mode = fuse.S_IFLNK | 0777
	n.link = content
	me.Inode().AddChild(name, n.Inode())
	me.touch()
	me.deleted[name] = false, false
	return &n.info, n, fuse.OK
}

func (me *memNode) Rename(oldName string, newParent fuse.FsNode, newName string, context *fuse.Context) (code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	ch := me.Inode().RmChild(oldName)
	me.deleted[oldName] = true
	newParent.Inode().RmChild(newName)
	newParent.Inode().AddChild(newName, ch)
	me.deleted[newName] = false, false
	me.touch()
	return fuse.OK
}

func (me *memNode) Link(name string, existing fuse.FsNode, context *fuse.Context) (fi *os.FileInfo, newNode fuse.FsNode, code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.Inode().AddChild(name, existing.Inode())
	fi, code = existing.GetAttr(nil, context)
	me.touch()
	me.deleted[name] = false, false
	return fi, existing, code
}

func (me *memNode) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, fi *os.FileInfo, newNode fuse.FsNode, code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	n := me.newNode(false)
	n.info.Mode = mode | fuse.S_IFREG
	n.changed = true
	n.backing = me.fs.getFilename()
	f, err := os.Create(n.backing)
	if err != nil {
		return nil, nil, nil, fuse.OsErrorToErrno(err)
	}
	me.Inode().AddChild(name, n.Inode())
	me.touch()
	me.deleted[name] = false, false
	return n.newFile(&fuse.LoopbackFile{File: f}, true), &n.info, n, fuse.OK
}

type memNodeFile struct {
	fuse.File
	writable bool
	node     *memNode
}

func (me *memNodeFile) InnerFile() fuse.File {
	return me.File
}

func (me *memNodeFile) Flush() fuse.Status {
	code := me.File.Flush()
	if me.writable {
		me.node.mutex.Lock()
		defer me.node.mutex.Unlock()
		fi, _ := me.File.GetAttr()
		me.node.info.Size = fi.Size
		me.node.info.Blocks = fi.Blocks
	}
	return code
}

func (me *memNode) newFile(f fuse.File, writable bool) fuse.File {
	return &memNodeFile{
		File:     f,
		writable: writable,
		node:     me,
	}
}

// Must run inside mutex.
func (me *memNode) promote() {
	if me.backing == "" {
		me.backing = me.fs.getFilename()
		destfs := &fuse.LoopbackFileSystem{Root: "/"}
		fuse.CopyFile(me.fs.readonly, destfs,
			me.original, strings.TrimLeft(me.backing, "/"), nil)

		files := me.Inode().Files(0)
		for _, f := range files {
			mf := f.File.(*memNodeFile)
			inner := mf.File
			osFile, err := os.Open(me.backing)
			if err != nil {
				panic("error opening backing file")
			}
			mf.File = &fuse.LoopbackFile{File: osFile}
			inner.Flush()
			inner.Release()
		}
	}
}

func (me *memNode) Open(flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		me.mutex.Lock()
		defer me.mutex.Unlock()

		me.promote()
		me.touch()
	}

	if me.backing != "" {
		f, err := os.OpenFile(me.backing, int(flags), 0666)
		if err != nil {
			return nil, fuse.OsErrorToErrno(err)
		}
		wr := flags&fuse.O_ANYWRITE != 0
		return me.newFile(&fuse.LoopbackFile{File: f}, wr), fuse.OK
	}

	file, code = me.fs.readonly.Open(me.original, flags, context)
	if !code.Ok() {
		return nil, code
	}

	return me.newFile(file, false), fuse.OK
}

func (me *memNode) GetAttr(file fuse.File, context *fuse.Context) (fi *os.FileInfo, code fuse.Status) {
	me.mutex.RLock()
	defer me.mutex.RUnlock()
	info := me.info
	if file != nil {
		fi, _ := file.GetAttr()
		info.Size = fi.Size
	}
	return &info, fuse.OK
}

func (me *memNode) Truncate(file fuse.File, size uint64, context *fuse.Context) (code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.promote()
	if file != nil {
		return file.Truncate(size)
	}

	me.info.Size = int64(size)
	err := os.Truncate(me.backing, int64(size))
	me.touch()
	return fuse.OsErrorToErrno(err)
}

func (me *memNode) Utimens(file fuse.File, atime uint64, mtime uint64, context *fuse.Context) (code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.info.Atime_ns = int64(atime)
	me.info.Mtime_ns = int64(mtime)
	me.ctouch()
	return fuse.OK
}

func (me *memNode) Chmod(file fuse.File, perms uint32, context *fuse.Context) (code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()

	me.info.Mode = (me.info.Mode &^ 07777) | perms
	me.ctouch()
	return fuse.OK
}

func (me *memNode) Chown(file fuse.File, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	if context.Uid != 0 {
		return fuse.EPERM
	}

	me.mutex.Lock()
	defer me.mutex.Unlock()

	me.info.Uid = int(uid)
	me.info.Gid = int(gid)
	me.ctouch()
	return fuse.OK
}

func (me *memNode) OpenDir(context *fuse.Context) (stream chan fuse.DirEntry, code fuse.Status) {
	me.mutex.RLock()
	defer me.mutex.RUnlock()
	ch := map[string]uint32{}

	if me.original != "" || me == me.fs.root {
		stream, code = me.fs.readonly.OpenDir(me.original, context)
		for e := range stream {
			ch[e.Name] = e.Mode
		}
	}

	for k, n := range me.Inode().FsChildren() {
		fi, code := n.FsNode().GetAttr(nil, nil)
		if !code.Ok() {
			panic("child does not have mode.")
		}
		ch[k] = fi.Mode
	}

	for k, _ := range me.deleted {
		ch[k] = 0, false
	}

	stream = make(chan fuse.DirEntry, len(ch))
	for k, v := range ch {
		stream <- fuse.DirEntry{Name: k, Mode: v}
	}
	close(stream)
	return stream, fuse.OK
}
