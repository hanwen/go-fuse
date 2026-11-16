// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package winfsp

import (
	"cmp"
	"fmt"
	"log"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/fuseclient"
)

// fileSystem implements the WinFsp file system callbacks
// (FSP_FILE_SYSTEM_INTERFACE) with FUSE requests. It acts as a small
// FUSE kernel: it resolves paths to node IDs and keeps lookup counts.
// It does not depend on WinFsp, so it can be tested on Linux.
//
// Paths use backslashes, and are relative to the volume root, eg.
// `\dir\file`.
type fileSystem struct {
	// tree is set once INIT has completed; notifications that
	// arrive during INIT read it with notifyTree.
	tree   *fuseclient.Tree
	opts   *fuse.MountOptions
	caller fuse.Caller

	// handles maps WinFsp FileContexts to open files.
	handlesMu  sync.Mutex
	handles    map[uintptr]*file
	nextHandle uintptr

	// notify is called (asynchronously) with a path when the file
	// system invalidates an entry or inode. It may be nil.
	notifyMu sync.Mutex
	notify   func(path string, c change)
	// pending holds notifications for the goroutine that sends
	// them, which runs if notifying is set.
	pending   []notification
	notifying bool
}

// change is the kind of change that a notification reports.
type change int

const (
	// changeInode: the attributes or data of a file changed.
	changeInode change = iota
	// changeEntry: a name may have appeared, changed or gone.
	changeEntry
	// changeRemoved: a name was removed.
	changeRemoved
)

type notification struct {
	path string
	c    change
}

// file is the WinFsp FileContext.
type file struct {
	node   *fuseclient.Node
	h      *fuseclient.Handle // nil for symlinks
	isDir  bool
	isLink bool
	isRoot bool

	mu  sync.Mutex
	dir []dirEntry
}

type dirEntry struct {
	name string
	info fileInfo
}

func newFileSystem(raw fuse.RawFileSystem, opts *fuse.MountOptions, caller fuse.Caller) (*fileSystem, error) {
	o := *opts
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	fsys := &fileSystem{
		opts:    &o,
		caller:  caller,
		handles: map[uintptr]*file{},
	}
	notify := fuse.Notify{
		InvalEntry: func(parent uint64, name string) syscall.Errno {
			fsys.invalEntry(parent, name, changeEntry)
			return 0
		},
		Delete: func(parent uint64, child uint64, name string) syscall.Errno {
			fsys.invalEntry(parent, name, changeRemoved)
			return 0
		},
		InvalInode: func(node uint64, off int64, length int64) syscall.Errno {
			if t := fsys.notifyTree(); t != nil {
				if p, ok := t.Path(node); ok {
					fsys.sendNotify(p, changeInode)
				}
			}
			return 0
		},
	}
	c, err := fuseclient.New(raw, opts, notify)
	if err != nil {
		return nil, err
	}
	fsys.notifyMu.Lock()
	fsys.tree = fuseclient.NewTree(c)
	fsys.notifyMu.Unlock()
	return fsys, nil
}

// notifyTree returns the tree for a notification handler, or nil if
// the notification arrived during INIT, when nothing is cached yet.
func (fsys *fileSystem) notifyTree() *fuseclient.Tree {
	fsys.notifyMu.Lock()
	defer fsys.notifyMu.Unlock()
	return fsys.tree
}

// addHandle registers an open file, and returns its FileContext,
// which is never 0.
func (fsys *fileSystem) addHandle(f *file) uintptr {
	fsys.handlesMu.Lock()
	defer fsys.handlesMu.Unlock()
	fsys.nextHandle++
	fsys.handles[fsys.nextHandle] = f
	return fsys.nextHandle
}

// handle returns the file for a FileContext, or nil.
func (fsys *fileSystem) handle(h uintptr) *file {
	fsys.handlesMu.Lock()
	defer fsys.handlesMu.Unlock()
	return fsys.handles[h]
}

// closeHandle closes the file for a FileContext.
func (fsys *fileSystem) closeHandle(h uintptr) {
	fsys.handlesMu.Lock()
	f := fsys.handles[h]
	delete(fsys.handles, h)
	fsys.handlesMu.Unlock()
	if f != nil {
		fsys.close(f)
	}
}

// shutdown is called after the last callback. It drops the open
// files; Tree.Close releases their handles, as the kernel does on
// unmount.
func (fsys *fileSystem) shutdown() {
	fsys.handlesMu.Lock()
	fsys.handles = map[uintptr]*file{}
	fsys.handlesMu.Unlock()
	fsys.tree.Close()
}

func (fsys *fileSystem) setNotify(f func(path string, c change)) {
	fsys.notifyMu.Lock()
	defer fsys.notifyMu.Unlock()
	fsys.notify = f
}

func (fsys *fileSystem) invalEntry(parent uint64, name string, c change) {
	t := fsys.notifyTree()
	if t == nil {
		return
	}
	p, ok := t.Path(parent)
	t.InvalidateID(parent, name)
	if ok {
		fsys.sendNotify(strings.TrimSuffix(p, "/")+"/"+name, c)
	}
}

// sendNotify queues a notification for WinFsp. Notifications are sent
// in order from a separate goroutine, because FspFileSystemNotify may
// block on operations that are waiting for this file system.
func (fsys *fileSystem) sendNotify(path string, c change) {
	fsys.notifyMu.Lock()
	defer fsys.notifyMu.Unlock()
	if fsys.notify == nil {
		return
	}
	fsys.pending = append(fsys.pending, notification{strings.ReplaceAll(path, "/", `\`), c})
	if !fsys.notifying {
		fsys.notifying = true
		go fsys.notifyLoop()
	}
}

func (fsys *fileSystem) notifyLoop() {
	for {
		fsys.notifyMu.Lock()
		if len(fsys.pending) == 0 || fsys.notify == nil {
			fsys.pending = nil
			fsys.notifying = false
			fsys.notifyMu.Unlock()
			return
		}
		n := fsys.pending[0]
		fsys.pending = fsys.pending[1:]
		f := fsys.notify
		fsys.notifyMu.Unlock()
		f(n.path, n.c)
	}
}

func posixPath(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}

// splitPath returns the parent directory and the last component.
func splitPath(p string) (string, string) {
	p = strings.TrimRight(posixPath(p), "/")
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

// walkStatus returns the status for a failed Walk. A missing parent,
// or a parent that is not a directory, is
// STATUS_OBJECT_PATH_NOT_FOUND, as Windows expects.
func walkStatus(missingParent bool, errno syscall.Errno) ntStatus {
	if errno == syscall.ENOENT && missingParent {
		return _STATUS_OBJECT_PATH_NOT_FOUND
	}
	if errno == syscall.ENOTDIR {
		return _STATUS_OBJECT_PATH_NOT_FOUND
	}
	return statusFromErrno(errno)
}

// pathInfo converts the attributes of the file at path. A symlink to
// a directory gets FILE_ATTRIBUTE_DIRECTORY, so Windows treats it as a
// directory symlink.
func (fsys *fileSystem) pathInfo(path string, a *fuse.Attr) fileInfo {
	fi := fileInfoFromAttr(a)
	if a.Mode&sIFMT != sIFLNK {
		return fi
	}
	n, target, errno := fsys.tree.WalkFollow(&fsys.caller, posixPath(path))
	if errno == 0 {
		fsys.tree.Unref(n)
		if target.Mode&sIFMT == sIFDIR {
			fi.FileAttributes |= _FILE_ATTRIBUTE_DIRECTORY
		}
	}
	return fi
}

// childInfo is pathInfo for the entry name in the directory dir.
func (fsys *fileSystem) childInfo(dir *file, name string, a *fuse.Attr) fileInfo {
	if a.Mode&sIFMT != sIFLNK {
		return fileInfoFromAttr(a)
	}
	p, ok := fsys.tree.Path(dir.node.ID)
	if !ok {
		// The directory is no longer reachable, so the target
		// cannot be resolved.
		return fileInfoFromAttr(a)
	}
	return fsys.pathInfo(strings.TrimSuffix(p, "/")+"/"+name, a)
}

// fileInfo is pathInfo for an open file.
func (fsys *fileSystem) fileInfo(f *file, a *fuse.Attr) fileInfo {
	if !f.isLink {
		return fileInfoFromAttr(a)
	}
	p, ok := fsys.tree.Path(f.node.ID)
	if !ok {
		// Deleted while open.
		return fileInfoFromAttr(a)
	}
	return fsys.pathInfo(p, a)
}

func (fsys *fileSystem) getVolumeInfo() (volumeInfo, ntStatus) {
	out, errno := fsys.tree.Statfs(&fsys.caller, fuse.FUSE_ROOT_ID)
	if errno != 0 {
		return volumeInfo{}, statusFromErrno(errno)
	}
	bs := uint64(out.Frsize)
	if bs == 0 {
		bs = uint64(out.Bsize)
	}
	label := fsys.opts.FsName
	if label == "" {
		label = fsys.opts.Name
	}
	return volumeInfo{
		TotalSize: out.Blocks * bs,
		FreeSize:  out.Bavail * bs,
		Label:     label,
	}, _STATUS_SUCCESS
}

// getSecurityByName returns the attributes of the file at path. The
// Windows side maps them to a security descriptor.
func (fsys *fileSystem) getSecurityByName(path string) (uint32, fuse.Attr, ntStatus) {
	n, attr, missingParent, errno := fsys.tree.Walk(&fsys.caller, posixPath(path))
	if errno != 0 {
		return 0, attr, walkStatus(missingParent, errno)
	}
	fsys.tree.Unref(n)
	fi := fsys.pathInfo(path, &attr)
	return fi.FileAttributes, attr, _STATUS_SUCCESS
}

// lookupDir resolves the parent directory of path. The caller must
// Unref the node.
func (fsys *fileSystem) lookupDir(path string) (*fuseclient.Node, string, ntStatus) {
	dirPath, name := splitPath(path)
	dir, attr, _, errno := fsys.tree.Walk(&fsys.caller, dirPath)
	if errno != 0 {
		return nil, "", walkStatus(true, errno)
	}
	if attr.Mode&sIFMT != sIFDIR {
		fsys.tree.Unref(dir)
		return nil, "", _STATUS_OBJECT_PATH_NOT_FOUND
	}
	return dir, name, _STATUS_SUCCESS
}

func (fsys *fileSystem) create(path string, createOptions, fileAttributes uint32, mode uint32) (*file, fileInfo, ntStatus) {
	dir, name, ntst := fsys.lookupDir(path)
	if ntst != _STATUS_SUCCESS {
		return nil, fileInfo{}, ntst
	}
	defer fsys.tree.Unref(dir)
	isDir := createOptions&_FILE_DIRECTORY_FILE != 0
	// Windows ignores FILE_ATTRIBUTE_READONLY on directories.
	if !isDir && fileAttributes != _INVALID_FILE_ATTRIBUTES {
		mode = readOnlyMode(mode, fileAttributes&_FILE_ATTRIBUTE_READONLY != 0)
	}

	f := &file{}
	var entry fuse.EntryOut
	var errno syscall.Errno
	if isDir {
		f.node, entry, errno = fsys.tree.Mkdir(&fsys.caller, dir, name, mode&07777)
		// entry.NodeId is set if MKDIR succeeded, but the entry
		// could not be recorded.
		if errno != 0 && entry.NodeId == 0 {
			return nil, fileInfo{}, statusFromErrno(errno)
		}
		if errno == 0 {
			f.isDir = true
			f.h, errno = fsys.tree.Opendir(&fsys.caller, f.node.ID)
			if errno != 0 {
				fsys.tree.Unref(f.node)
			}
		}
		if errno != 0 {
			// Windows sees a failed create, so do not leave
			// the directory behind.
			fsys.tree.Rmdir(&fsys.caller, dir.ID, name)
			fsys.tree.Invalidate(dir, name)
			return nil, fileInfo{}, statusFromErrno(errno)
		}
	} else {
		f.node, entry, f.h, errno = fsys.tree.Create(&fsys.caller, dir, name, syscall.O_CREAT|syscall.O_EXCL|syscall.O_RDWR, mode&07777)
		if errno != 0 {
			return nil, fileInfo{}, statusFromErrno(errno)
		}
	}
	return f, fileInfoFromAttr(&entry.Attr), _STATUS_SUCCESS
}

func openFlags(grantedAccess uint32) uint32 {
	read := grantedAccess&(_FILE_READ_DATA|_FILE_EXECUTE) != 0
	write := grantedAccess&(_FILE_WRITE_DATA|_FILE_APPEND_DATA) != 0
	switch {
	case read && write:
		return syscall.O_RDWR
	case write:
		return syscall.O_WRONLY
	}
	return syscall.O_RDONLY
}

func (fsys *fileSystem) open(path string, grantedAccess uint32) (*file, fileInfo, ntStatus) {
	n, attr, missingParent, errno := fsys.tree.Walk(&fsys.caller, posixPath(path))
	if errno != 0 {
		return nil, fileInfo{}, walkStatus(missingParent, errno)
	}
	f := &file{node: n, isRoot: n == fsys.tree.Root()}
	switch attr.Mode & sIFMT {
	case sIFDIR:
		f.isDir = true
		f.h, errno = fsys.tree.Opendir(&fsys.caller, n.ID)
	case sIFLNK:
		// Symlinks are reparse points, and are not opened.
		f.isLink = true
	default:
		f.h, errno = fsys.tree.Open(&fsys.caller, n.ID, openFlags(grantedAccess))
	}
	if errno != 0 {
		fsys.tree.Unref(n)
		return nil, fileInfo{}, statusFromErrno(errno)
	}
	return f, fsys.pathInfo(path, &attr), _STATUS_SUCCESS
}

// attrHandle returns the handle to pass with GETATTR and SETATTR,
// which the kernel only does for files.
func (f *file) attrHandle() *fuseclient.Handle {
	if f.isDir {
		return nil
	}
	return f.h
}

func (fsys *fileSystem) getattr(f *file) (fuse.Attr, ntStatus) {
	out, errno := fsys.tree.Getattr(&fsys.caller, f.node.ID, f.attrHandle())
	return out.Attr, statusFromErrno(errno)
}

func (fsys *fileSystem) getFileInfo(f *file) (fileInfo, ntStatus) {
	a, st := fsys.getattr(f)
	if st != _STATUS_SUCCESS {
		return fileInfo{}, st
	}
	return fsys.fileInfo(f, &a), _STATUS_SUCCESS
}

func (fsys *fileSystem) setattr(f *file, in *fuse.SetAttrIn) (fileInfo, ntStatus) {
	out, errno := fsys.tree.Setattr(&fsys.caller, f.node.ID, f.attrHandle(), in)
	if errno != 0 {
		return fileInfo{}, statusFromErrno(errno)
	}
	return fsys.fileInfo(f, &out.Attr), _STATUS_SUCCESS
}

func (fsys *fileSystem) overwrite(f *file, fileAttributes uint32, replaceAttributes bool) (fileInfo, ntStatus) {
	if f.isDir || f.isLink {
		return fileInfo{}, _STATUS_ACCESS_DENIED
	}
	in := fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	if fileAttributes == _INVALID_FILE_ATTRIBUTES {
		fileAttributes = 0
	}
	// Without replaceAttributes, the new attributes are added to
	// the existing ones.
	readOnly := fileAttributes&_FILE_ATTRIBUTE_READONLY != 0
	if readOnly || replaceAttributes {
		a, st := fsys.getattr(f)
		if st != _STATUS_SUCCESS {
			return fileInfo{}, st
		}
		if mode := readOnlyMode(a.Mode&07777, readOnly); mode != a.Mode&07777 {
			in.Valid |= fuse.FATTR_MODE
			in.Mode = mode
		}
	}
	return fsys.setattr(f, &in)
}

// readOnlyMode applies FILE_ATTRIBUTE_READONLY to permission bits:
// setting it clears all write bits, and clearing it makes the file
// writable by the owner.
func readOnlyMode(mode uint32, readOnly bool) uint32 {
	if readOnly {
		return mode &^ 0222
	}
	if mode&0222 == 0 {
		return mode | 0200
	}
	return mode
}

func (fsys *fileSystem) cleanup(f *file, path string, flags uint32) {
	if flags&cleanupDelete == 0 || path == "" {
		return
	}
	dir, name, ntst := fsys.lookupDir(path)
	if ntst != _STATUS_SUCCESS {
		return
	}
	defer fsys.tree.Unref(dir)
	var errno syscall.Errno
	if f.isDir {
		errno = fsys.tree.Rmdir(&fsys.caller, dir.ID, name)
	} else {
		errno = fsys.tree.Unlink(&fsys.caller, dir.ID, name)
	}
	if errno == 0 {
		fsys.tree.Invalidate(dir, name)
	} else if fsys.opts.Debug {
		fsys.opts.Logger.Printf("winfsp: delete %q: %v", path, errno)
	}
}

func (fsys *fileSystem) close(f *file) {
	if f.h != nil {
		if !f.isDir {
			fsys.tree.Flush(&fsys.caller, f.h)
		}
		fsys.tree.Release(f.h)
	}
	fsys.tree.Unref(f.node)
}

func (fsys *fileSystem) read(f *file, buf []byte, off uint64) (int, ntStatus) {
	if f.isDir || f.isLink {
		return 0, _STATUS_INVALID_DEVICE_REQUEST
	}
	n, errno := fsys.tree.Read(&fsys.caller, f.h, off, buf)
	if errno != 0 {
		return 0, statusFromErrno(errno)
	}
	if n == 0 && len(buf) > 0 {
		return 0, _STATUS_END_OF_FILE
	}
	return n, _STATUS_SUCCESS
}

// write writes buf at off. It sends one GETATTR, before the WRITE:
// appending and constrained writes need the size, and WinFsp wants
// the file info back. Afterwards the size is updated locally, so the
// times in the returned info are from before the write. WinFsp's FUSE
// layer does the same.
//
// TODO: the kernel needs no GETATTR here. It caches attributes for the
// file system's AttrValid timeout, and after a WRITE updates the
// cached size and marks the times stale (fuse_write_update_attr). An
// attribute cache in fuseclient.Tree would do the same, and also save
// the GETATTR that Walk and getFileInfo send for cached entries.
func (fsys *fileSystem) write(f *file, buf []byte, off uint64, toEnd, constrained bool) (int, fileInfo, ntStatus) {
	if f.isDir || f.isLink {
		return 0, fileInfo{}, _STATUS_INVALID_DEVICE_REQUEST
	}
	a, st := fsys.getattr(f)
	if st != _STATUS_SUCCESS {
		return 0, fileInfo{}, st
	}
	if toEnd {
		off = a.Size
	}
	if constrained {
		if off >= a.Size {
			return 0, fileInfoFromAttr(&a), _STATUS_SUCCESS
		}
		buf = buf[:min(uint64(len(buf)), a.Size-off)]
	}
	n, errno := fsys.tree.Write(&fsys.caller, f.h, off, buf)
	if errno != 0 {
		return 0, fileInfo{}, statusFromErrno(errno)
	}
	a.Size = max(a.Size, off+uint64(n))
	return n, fileInfoFromAttr(&a), _STATUS_SUCCESS
}

func (fsys *fileSystem) flush(f *file) (fileInfo, ntStatus) {
	if f == nil {
		return fileInfo{}, _STATUS_SUCCESS
	}
	if !f.isDir && !f.isLink {
		if errno := fsys.tree.Fsync(&fsys.caller, f.h); errno != 0 && errno != syscall.ENOSYS {
			return fileInfo{}, statusFromErrno(errno)
		}
	}
	return fsys.getFileInfo(f)
}

// isSet reports whether a FILETIME argument asks for a change. Zero
// means "no change", and -1 means "stop updating" (which we ignore).
func isSet(ft uint64) bool {
	return ft != 0 && ft != ^uint64(0)
}

func (fsys *fileSystem) setBasicInfo(f *file, fileAttributes uint32, creationTime, lastAccessTime, lastWriteTime, changeTime uint64) (fileInfo, ntStatus) {
	in := fuse.SetAttrIn{}
	if isSet(lastAccessTime) {
		t := unixTime(lastAccessTime)
		in.Valid |= fuse.FATTR_ATIME
		in.Atime = uint64(t.Unix())
		in.Atimensec = uint32(t.Nanosecond())
	}
	if isSet(lastWriteTime) {
		t := unixTime(lastWriteTime)
		in.Valid |= fuse.FATTR_MTIME
		in.Mtime = uint64(t.Unix())
		in.Mtimensec = uint32(t.Nanosecond())
	}
	// INVALID_FILE_ATTRIBUTES means no change; the WinFsp driver
	// sends it for 0, so 0 here clears all attributes. Windows
	// ignores FILE_ATTRIBUTE_READONLY on directories, and a POSIX
	// symlink has no permission bits to keep it in.
	if fileAttributes != _INVALID_FILE_ATTRIBUTES && !f.isDir && !f.isLink {
		a, st := fsys.getattr(f)
		if st != _STATUS_SUCCESS {
			return fileInfo{}, st
		}
		if mode := readOnlyMode(a.Mode&07777, fileAttributes&_FILE_ATTRIBUTE_READONLY != 0); mode != a.Mode&07777 {
			in.Valid |= fuse.FATTR_MODE
			in.Mode = mode
		}
	}
	if in.Valid == 0 {
		return fsys.getFileInfo(f)
	}
	return fsys.setattr(f, &in)
}

func (fsys *fileSystem) setFileSize(f *file, newSize uint64, setAllocationSize bool) (fileInfo, ntStatus) {
	if f.isDir || f.isLink {
		return fileInfo{}, _STATUS_INVALID_DEVICE_REQUEST
	}
	if setAllocationSize {
		// FUSE has no allocation size. Only shrink the file
		// if the allocation is smaller than the size.
		//
		// TODO: reserve space for a larger allocation with
		// FALLOCATE and FALLOC_FL_KEEP_SIZE, ignoring ENOSYS
		// and EOPNOTSUPP.
		a, st := fsys.getattr(f)
		if st != _STATUS_SUCCESS {
			return fileInfo{}, st
		}
		if a.Size <= newSize {
			return fileInfoFromAttr(&a), _STATUS_SUCCESS
		}
	}
	in := fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	in.Size = newSize
	return fsys.setattr(f, &in)
}

func (fsys *fileSystem) canDelete(f *file) ntStatus {
	if f.isRoot {
		return _STATUS_ACCESS_DENIED
	}
	if !f.isDir {
		return _STATUS_SUCCESS
	}
	// Reading from offset 0 rewinds the handle, so do not
	// interleave with readDirectory.
	f.mu.Lock()
	defer f.mu.Unlock()
	var off uint64
	for {
		ents, errno := fsys.tree.Readdir(&fsys.caller, f.node, f.h, off, false)
		if errno != 0 {
			return statusFromErrno(errno)
		}
		if len(ents) == 0 {
			return _STATUS_SUCCESS
		}
		for _, e := range ents {
			if e.Name != "." && e.Name != ".." {
				return _STATUS_DIRECTORY_NOT_EMPTY
			}
			off = e.Off
		}
	}
}

func (fsys *fileSystem) rename(path, newPath string, replaceIfExists bool) ntStatus {
	n, attr, _, errno := fsys.tree.Walk(&fsys.caller, posixPath(newPath))
	if errno == 0 {
		fsys.tree.Unref(n)
		if !replaceIfExists {
			return _STATUS_OBJECT_NAME_COLLISION
		}
		if attr.Mode&sIFMT == sIFDIR {
			return _STATUS_ACCESS_DENIED
		}
	}

	dir, name, ntst := fsys.lookupDir(path)
	if ntst != _STATUS_SUCCESS {
		return ntst
	}
	defer fsys.tree.Unref(dir)
	newDir, newName, ntst := fsys.lookupDir(newPath)
	if ntst != _STATUS_SUCCESS {
		return ntst
	}
	defer fsys.tree.Unref(newDir)

	errno = fsys.tree.Rename(&fsys.caller, dir.ID, name, newDir.ID, newName)
	if errno != 0 {
		return statusFromErrno(errno)
	}
	fsys.tree.Move(dir, name, newDir, newName)
	return _STATUS_SUCCESS
}

// dirRank orders "." and ".." before all other names, as NTFS and
// WinFsp's directory buffers do; some programs skip the first two
// entries of a listing.
func dirRank(name string) int {
	switch name {
	case ".":
		return 0
	case "..":
		return 1
	}
	return 2
}

// compareDirNames is the order of a directory listing.
func compareDirNames(a, b string) int {
	if c := cmp.Compare(dirRank(a), dirRank(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// loadDir reads the whole directory, in compareDirNames order.
func (fsys *fileSystem) loadDir(f *file) ntStatus {
	var res []dirEntry
	var off uint64
	for {
		// Readdir records the READDIRPLUS entries in the tree.
		ents, errno := fsys.tree.Readdir(&fsys.caller, f.node, f.h, off, true)
		if errno != 0 {
			return statusFromErrno(errno)
		}
		if len(ents) == 0 {
			break
		}
		for i := range ents {
			e := &ents[i]
			off = e.Off
			if e.Name == "." || e.Name == ".." {
				// "." and ".." do not count as lookups.
				continue
			}
			attr := e.Entry.Attr
			if e.Entry.NodeId == 0 {
				n, out, errno := fsys.tree.Lookup(&fsys.caller, f.node, e.Name)
				if errno != 0 {
					// Raced with a removal.
					continue
				}
				fsys.tree.Unref(n)
				attr = out.Attr
			}
			res = append(res, dirEntry{e.Name, fsys.childInfo(f, e.Name, &attr)})
		}
	}
	if !f.isRoot {
		// Windows lists "." and ".." except in the root.
		a, ntst := fsys.getattr(f)
		if ntst != _STATUS_SUCCESS {
			return ntst
		}
		fi := fsys.fileInfo(f, &a)
		res = append(res, dirEntry{".", fi}, dirEntry{"..", fi})
	}
	slices.SortFunc(res, func(a, b dirEntry) int { return compareDirNames(a.name, b.name) })
	f.dir = res
	return _STATUS_SUCCESS
}

// readDirectory fills buf with FSP_FSCTL_DIR_INFO records for the
// entries after marker. A nil marker starts a new listing.
func (fsys *fileSystem) readDirectory(f *file, marker *string, buf []byte) (int, ntStatus) {
	if !f.isDir {
		return 0, _STATUS_NOT_A_DIRECTORY
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if marker == nil || f.dir == nil {
		if st := fsys.loadDir(f); st != _STATUS_SUCCESS {
			return 0, st
		}
	}
	start := 0
	if marker != nil {
		start, _ = slices.BinarySearchFunc(f.dir, *marker, func(e dirEntry, m string) int {
			return compareDirNames(e.name, m)
		})
		if start < len(f.dir) && f.dir[start].name == *marker {
			start++
		}
	}
	off := 0
	for _, e := range f.dir[start:] {
		var ok bool
		off, ok = appendDirInfo(buf, off, e.name, &e.info)
		if !ok {
			return off, _STATUS_SUCCESS
		}
	}
	if off+2 <= len(buf) {
		buf[off] = 0
		buf[off+1] = 0
		off += 2
	}
	return off, _STATUS_SUCCESS
}

func (fsys *fileSystem) getDirInfoByName(f *file, name string) (fileInfo, ntStatus) {
	if name == "." || name == ".." {
		// As in loadDir, which lists them outside the root.
		if f.isRoot {
			return fileInfo{}, _STATUS_OBJECT_NAME_NOT_FOUND
		}
		return fsys.getFileInfo(f)
	}
	n, out, errno := fsys.tree.Lookup(&fsys.caller, f.node, name)
	if errno != 0 {
		return fileInfo{}, statusFromErrno(errno)
	}
	fsys.tree.Unref(n)
	return fsys.childInfo(f, name, &out.Attr), _STATUS_SUCCESS
}

func (fsys *fileSystem) getSecurity(f *file) (fuse.Attr, ntStatus) {
	return fsys.getattr(f)
}

// setSecurity changes the permission bits, and owner if uid or gid are
// not nil.
func (fsys *fileSystem) setSecurity(f *file, mode uint32, uid, gid *uint32) ntStatus {
	in := fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_MODE
	in.Mode = mode & 07777
	if uid != nil {
		in.Valid |= fuse.FATTR_UID
		in.Uid = *uid
	}
	if gid != nil {
		in.Valid |= fuse.FATTR_GID
		in.Gid = *gid
	}
	_, st := fsys.setattr(f, &in)
	return st
}

// setSymlink turns the file or empty directory f at path into a
// symlink to target. Windows creates a symlink by creating a file and
// then setting a reparse point on it, which has no FUSE equivalent. As
// in WinFsp's FUSE layer, we create the symlink under a temporary
// name, and rename it over path; the inode number changes. The handle
// f keeps referring to the replaced file.
func (fsys *fileSystem) setSymlink(f *file, path, target string) ntStatus {
	if f.isRoot {
		return _STATUS_ACCESS_DENIED
	}
	dir, name, st := fsys.lookupDir(path)
	if st != _STATUS_SUCCESS {
		return st
	}
	defer fsys.tree.Unref(dir)

	tmp := fmt.Sprintf(".winfsp-symlink-%016x", rand.Uint64())
	n, entry, errno := fsys.tree.Symlink(&fsys.caller, dir, tmp, target)
	if errno != 0 {
		if entry.NodeId != 0 {
			// SYMLINK succeeded, but the entry could not
			// be recorded.
			fsys.tree.Unlink(&fsys.caller, dir.ID, tmp)
		}
		return statusFromErrno(errno)
	}
	defer fsys.tree.Unref(n)
	if f.isDir {
		// RENAME does not replace a directory with a symlink.
		errno = fsys.tree.Rmdir(&fsys.caller, dir.ID, name)
		if errno == 0 {
			fsys.tree.Invalidate(dir, name)
		}
	}
	if errno == 0 {
		errno = fsys.tree.Rename(&fsys.caller, dir.ID, tmp, dir.ID, name)
	}
	if errno != 0 {
		fsys.tree.Unlink(&fsys.caller, dir.ID, tmp)
		fsys.tree.Invalidate(dir, tmp)
		return statusFromErrno(errno)
	}
	fsys.tree.Move(dir, tmp, dir, name)
	return _STATUS_SUCCESS
}

// readlink returns the target of the symlink at path.
func (fsys *fileSystem) readlink(path string) (string, ntStatus) {
	n, attr, missingParent, errno := fsys.tree.Walk(&fsys.caller, posixPath(path))
	if errno != 0 {
		return "", walkStatus(missingParent, errno)
	}
	defer fsys.tree.Unref(n)
	if attr.Mode&sIFMT != sIFLNK {
		return "", _STATUS_NOT_A_REPARSE_POINT
	}
	target, errno := fsys.tree.Readlink(&fsys.caller, n.ID)
	return target, statusFromErrno(errno)
}
