package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sort"
	"sync"
	"time"
)

var _ = log.Println

// A unionfs that only uses on-disk backing store for file contents.
type MemUnionFs struct {
	fuse.DefaultNodeFileSystem
	backingStore string
	root         *memNode
	connector    *fuse.FileSystemConnector
	mutex    sync.RWMutex
	cond     *sync.Cond
	nextFree int

	readonly fuse.FileSystem

	openWritable int
}

type memNode struct {
	fuse.DefaultFsNode
	fs *MemUnionFs

	// protects mutable data below.
	mutex    *sync.RWMutex
	backing  string
	original string
	changed  bool
	link     string
	info     os.FileInfo
	deleted  map[string]bool
}

type Result struct {
	*os.FileInfo
	Original string
	Backing  string
	Link     string
}

func (me *MemUnionFs) OnMount(conn *fuse.FileSystemConnector) {
	me.connector = conn
}

func (me *MemUnionFs) release() {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.openWritable--
	me.cond.Broadcast()
}

func (me *MemUnionFs) Reap() map[string]*Result {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	for me.openWritable > 0 {
		me.cond.Wait()
	}
	
	m := map[string]*Result{}
	me.root.Reap("", m)
	return m
}

func (me *MemUnionFs) Clear() {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.root.Clear("")
}

func (me *MemUnionFs) Update(results map[string]*Result) {
	del := []string{}
	add := []string{}
	for k, v := range results {
		if v.FileInfo != nil {
			add = append(add, k)
		} else {
			del = append(del, k)
		}
	}

	sort.Strings(del)
	for i := len(del)-1; i >= 0; i-- {
		n := del[i]
		dir, base := filepath.Split(n)
		dir = strings.TrimRight(dir, "/")
		dirNode, rest := me.connector.Node(me.root.Inode(), dir)
		if len(rest) > 0 {
			continue
		}
		
		dirNode.RmChild(base)
		me.connector.EntryNotify(dirNode, base)
	}
	
	me.mutex.Lock()
	defer me.mutex.Unlock()
	
	sort.Strings(add)
	for _, n := range add {
		node, rest := me.connector.Node(me.root.Inode(), n)
		if len(rest) > 0 {
			me.connector.EntryNotify(node, rest[0])
			continue
		}
		me.connector.FileNotify(node, 0, 0)
		mn := node.FsNode().(*memNode)
		mn.original = n
		mn.changed = false

		r := results[n]
		mn.info = *r.FileInfo
		mn.link = r.Link
	}
}

func (me *MemUnionFs) getFilename() string {
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
		fs:    me,
		mutex: &me.mutex,
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
	me.cond = sync.NewCond(&me.mutex)
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
	n.changed = true
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
	me.Inode().AddChild(name, existing.Inode())
	fi, code = existing.GetAttr(nil, context)

	me.mutex.Lock()
	defer me.mutex.Unlock()
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
	me.fs.openWritable++
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

func (me *memNodeFile) Release() {
	me.node.fs.release()
	me.File.Release()
}

func (me *memNodeFile) Flush() fuse.Status {
	code := me.File.Flush()
	if me.writable {
		fi, _ := me.File.GetAttr()

		me.node.mutex.Lock()
		defer me.node.mutex.Unlock()
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
		me.original = ""
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
	me.mutex.Lock()
	defer me.mutex.Unlock()
	if flags&fuse.O_ANYWRITE != 0 {
		me.promote()
		me.touch()
	}

	if me.backing != "" {
		f, err := os.OpenFile(me.backing, int(flags), 0666)
		if err != nil {
			return nil, fuse.OsErrorToErrno(err)
		}
		wr := flags&fuse.O_ANYWRITE != 0
		if wr {
			me.fs.openWritable++
		}
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

func (me *memNode) Reap(path string, results map[string]*Result) {
	for name, _ := range me.deleted {
		p := filepath.Join(path, name)
		results[p] = &Result{}
	}

	if me.changed {
		info := me.info
		results[path] = &Result{
			FileInfo:     &info,
			Link:     me.link,
			Backing:  me.backing,
			Original: me.original,
		}
	}

	for n, ch := range me.Inode().FsChildren() {
		p := filepath.Join(path, n)
		ch.FsNode().(*memNode).Reap(p, results)
	}
}

func (me *memNode) Clear(path string) {
	me.original = path
	me.changed = false
	me.backing = ""
	me.deleted = make(map[string]bool)
	for n, ch := range me.Inode().FsChildren() {
		p := filepath.Join(path, n)
		mn := ch.FsNode().(*memNode)
		mn.Clear(p)
	}
}
