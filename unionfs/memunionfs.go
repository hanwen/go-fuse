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
	mutex        sync.RWMutex
	cond         *sync.Cond
	nextFree     int

	readonly fuse.FileSystem

	openWritable int

	// All paths that have been renamed or deleted will be marked
	// here.  After deletion, entries may be recreated, but they
	// will be treated as new.
	deleted map[string]bool
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

	for name, _ := range me.deleted {

		fi, code := me.readonly.GetAttr(name, nil)
		if !code.Ok() {
			continue
		}
		m[name] = &Result{}
		if !fi.IsDirectory() {
			continue
		}

		todo := []string{name}
		for len(todo) > 0 {
			l := len(todo) - 1
			n := todo[l]
			todo = todo[:l]

			s, _ := me.readonly.OpenDir(n, nil)
			for e := range s {
				full := filepath.Join(n, e.Name)
				m[full] = &Result{}
				if e.Mode&fuse.S_IFDIR != 0 {
					todo = append(todo, full)
				}
			}
		}
	}

	me.root.Reap("", m)
	return m
}

func (me *MemUnionFs) Clear() {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.deleted = make(map[string]bool)
	me.root.Clear("")
	f, err := os.Open(me.backingStore)
	if err != nil {
		return
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return
	}
	for _, n := range names {
		os.Remove(filepath.Join(me.backingStore, n))
	}
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
	for i := len(del) - 1; i >= 0; i-- {
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
	notifyNodes := []*fuse.Inode{}
	enotifyNodes := []*fuse.Inode{}
	enotifyNames := []string{}

	sort.Strings(add)
	for _, n := range add {
		node, rest := me.connector.Node(me.root.Inode(), n)
		if len(rest) > 0 {
			enotifyNames = append(enotifyNames, rest[0])
			enotifyNodes = append(enotifyNodes, node)
			continue
		}
		notifyNodes = append(notifyNodes, node)
		mn := node.FsNode().(*memNode)
		mn.original = n
		mn.changed = false

		r := results[n]
		mn.info = *r.FileInfo
		mn.link = r.Link
	}
	me.mutex.Unlock()

	for _, n := range notifyNodes {
		me.connector.FileNotify(n, 0, 0)
	}
	for i, n := range enotifyNodes {
		me.connector.EntryNotify(n, enotifyNames[i])
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

func (me *MemUnionFs) newNode(isdir bool) *memNode {
	n := &memNode{
		fs:    me,
		mutex: &me.mutex,
	}
	now := time.Nanoseconds()
	n.info.Mtime_ns = now
	n.info.Atime_ns = now
	n.info.Ctime_ns = now
	return n
}

func NewMemUnionFs(backingStore string, roFs fuse.FileSystem) *MemUnionFs {
	me := &MemUnionFs{}
	me.deleted = make(map[string]bool)
	me.backingStore = backingStore
	me.readonly = roFs
	me.root = me.newNode(true)
	me.root.info.Mode = fuse.S_IFDIR | 0755
	me.cond = sync.NewCond(&me.mutex)

	return me
}

func (me *memNode) Deletable() bool {
	return !me.changed && me.original == ""
}

func (me *memNode) StatFs() *fuse.StatfsOut {
	backingFs := &fuse.LoopbackFileSystem{Root: me.fs.backingStore}
	return backingFs.StatFs("")
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
	return me.lookup(name, context)
}

// Must run with mutex held.
func (me *memNode) lookup(name string, context *fuse.Context) (fi *os.FileInfo, node fuse.FsNode, code fuse.Status) {
	if me.original == "" && me != me.fs.root {
		return nil, nil, fuse.ENOENT
	}

	fn := filepath.Join(me.original, name)
	if _, del := me.fs.deleted[fn]; del {
		return nil, nil, fuse.ENOENT
	}

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

	if me.original != "" || me == me.fs.root {
		me.fs.deleted[filepath.Join(me.original, name)] = true
	}
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
	return &n.info, n, fuse.OK
}

// Expand the original fs as a tree.
func (me *memNode) materializeSelf() {
	me.changed = true
	if !me.Inode().IsDir() {
		return
	}
	s, _ := me.fs.readonly.OpenDir(me.original, nil)
	for e := range s {
		me.lookup(e.Name, nil)
	}
	me.original = ""
}

func (me *memNode) materialize() {
	me.materializeSelf()
	for _, n := range me.Inode().FsChildren() {
		if n.IsDir() {
			n.FsNode().(*memNode).materialize()
		}
	}
}

func (me *memNode) Rename(oldName string, newParent fuse.FsNode, newName string, context *fuse.Context) (code fuse.Status) {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	ch := me.Inode().RmChild(oldName)
	if ch == nil {
		return fuse.ENOENT
	}

	if me.original != "" || me == me.fs.root {
		me.fs.deleted[filepath.Join(me.original, oldName)] = true
	}

	mn := ch.FsNode().(*memNode)
	if mn.original != "" || mn == me.fs.root {
		if newParent.(*memNode).original != "" {
			me.fs.deleted[filepath.Join(newParent.(*memNode).original, newName)] = true
		}

		mn.materialize()
		mn.markChanged()
	}

	newParent.Inode().RmChild(newName)
	newParent.Inode().AddChild(newName, ch)
	me.touch()
	return fuse.OK
}

func (me *memNode) markChanged() {
	me.changed = true
	for _, n := range me.Inode().FsChildren() {
		n.FsNode().(*memNode).markChanged()
	}
}

func (me *memNode) Link(name string, existing fuse.FsNode, context *fuse.Context) (fi *os.FileInfo, newNode fuse.FsNode, code fuse.Status) {
	me.Inode().AddChild(name, existing.Inode())
	fi, code = existing.GetAttr(nil, context)

	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.touch()
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
		log.Printf("Backing store error %q: %v", n.backing, err)
		return nil, nil, nil, fuse.OsErrorToErrno(err)
	}
	me.Inode().AddChild(name, n.Inode())
	me.touch()
	me.fs.openWritable++
	return n.newFile(&fuse.LoopbackFile{File: f}, true), &n.info, n, fuse.OK
}

type memNodeFile struct {
	fuse.File
	writable bool
	node     *memNode
}

func (me *memNodeFile) String() string {
	return fmt.Sprintf("memUfsFile(%s)", me.File.String())
}

func (me *memNodeFile) InnerFile() fuse.File {
	return me.File
}

func (me *memNodeFile) Release() {
	// Must do the subfile release first, as that may flush data
	// to disk.
	me.File.Release()
	me.node.fs.release()
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
	var sz int64
	if file != nil {
		fi, code := file.GetAttr()
		if code.Ok() {
			sz = fi.Size
		} else {
			msg := fmt.Sprintf("File.GetAttr(%s) = %v, %v", file.String(), fi, code)
			panic(msg)
		}
	}
	me.mutex.RLock()
	defer me.mutex.RUnlock()
	info := me.info
	if file != nil {
		info.Size = sz
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
			fn := filepath.Join(me.original, e.Name)
			if !me.fs.deleted[fn] {
				ch[e.Name] = e.Mode
			}
		}
	}

	for k, n := range me.Inode().FsChildren() {
		ch[k] = n.FsNode().(*memNode).info.Mode
	}

	stream = make(chan fuse.DirEntry, len(ch))
	for k, v := range ch {
		stream <- fuse.DirEntry{Name: k, Mode: v}
	}
	close(stream)
	return stream, fuse.OK
}

func (me *memNode) Reap(path string, results map[string]*Result) {
	if me.changed {
		info := me.info
		results[path] = &Result{
			FileInfo: &info,
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

func (me *memNode) markUsed(seen map[string]bool) {
	if me.backing != "" {
		seen[me.backing] = true
	}

	for _, ch := range me.Inode().FsChildren() {
		ch.FsNode().(*memNode).markUsed(seen)
	}
}

func (me *memNode) Clear(path string) {
	me.original = path
	me.changed = false
	me.backing = ""
	for n, ch := range me.Inode().FsChildren() {
		p := filepath.Join(path, n)
		mn := ch.FsNode().(*memNode)
		mn.Clear(p)
	}
}
