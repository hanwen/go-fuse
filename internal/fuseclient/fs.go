// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin

package fuseclient

import (
	"io"
	"io/fs"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
)

// FS reads the file system behind a Tree through the io/fs
// interfaces. Open and Stat follow symlinks with Tree.WalkFollow;
// Lstat and ReadLink do not follow the last component.
type FS struct {
	tree   *Tree
	caller fuse.Caller
}

var (
	_ fs.FS         = (*FS)(nil)
	_ fs.StatFS     = (*FS)(nil)
	_ fs.ReadLinkFS = (*FS)(nil)
)

// NewFS returns an FS that sends requests as caller.
func NewFS(t *Tree, caller fuse.Caller) *FS {
	return &FS{tree: t, caller: caller}
}

func pathError(op, name string, errno syscall.Errno) error {
	return &fs.PathError{Op: op, Path: name, Err: errno}
}

func (fsys *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	n, attr, errno := fsys.tree.WalkFollow(&fsys.caller, name)
	if errno != 0 {
		return nil, pathError("open", name, errno)
	}
	info := &fileInfo{name: path.Base(name), attr: attr}
	if attr.Mode&S_IFMT == fuse.S_IFDIR {
		h, errno := fsys.tree.Opendir(&fsys.caller, n.ID)
		if errno != 0 {
			fsys.tree.Unref(n)
			return nil, pathError("open", name, errno)
		}
		return &dir{fsys: fsys, node: n, h: h, name: name, info: info}, nil
	}
	h, errno := fsys.tree.Client.Open(&fsys.caller, n.ID, syscall.O_RDONLY)
	if errno != 0 {
		fsys.tree.Unref(n)
		return nil, pathError("open", name, errno)
	}
	return &file{fsys: fsys, node: n, h: h, name: name}, nil
}

func (fsys *FS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	n, attr, errno := fsys.tree.WalkFollow(&fsys.caller, name)
	if errno != 0 {
		return nil, pathError("stat", name, errno)
	}
	fsys.tree.Unref(n)
	return &fileInfo{name: path.Base(name), attr: attr}, nil
}

// lstat returns the node for name, following symlinks except in the
// last component.
func (fsys *FS) lstat(op, name string) (*Node, fuse.Attr, error) {
	if !fs.ValidPath(name) {
		return nil, fuse.Attr{}, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		n, attr, errno := fsys.tree.WalkFollow(&fsys.caller, name)
		if errno != 0 {
			return nil, attr, pathError(op, name, errno)
		}
		return n, attr, nil
	}
	d, dattr, errno := fsys.tree.WalkFollow(&fsys.caller, path.Dir(name))
	if errno != 0 {
		return nil, fuse.Attr{}, pathError(op, name, errno)
	}
	defer fsys.tree.Unref(d)
	if dattr.Mode&S_IFMT != fuse.S_IFDIR {
		return nil, fuse.Attr{}, pathError(op, name, syscall.ENOTDIR)
	}
	n, out, errno := fsys.tree.Lookup(&fsys.caller, d, path.Base(name))
	if errno != 0 {
		return nil, fuse.Attr{}, pathError(op, name, errno)
	}
	return n, out.Attr, nil
}

func (fsys *FS) Lstat(name string) (fs.FileInfo, error) {
	n, attr, err := fsys.lstat("lstat", name)
	if err != nil {
		return nil, err
	}
	fsys.tree.Unref(n)
	return &fileInfo{name: path.Base(name), attr: attr}, nil
}

func (fsys *FS) ReadLink(name string) (string, error) {
	n, attr, err := fsys.lstat("readlink", name)
	if err != nil {
		return "", err
	}
	defer fsys.tree.Unref(n)
	if attr.Mode&S_IFMT != fuse.S_IFLNK {
		return "", pathError("readlink", name, syscall.EINVAL)
	}
	target, errno := fsys.tree.Readlink(&fsys.caller, n.ID)
	if errno != 0 {
		return "", pathError("readlink", name, errno)
	}
	return target, nil
}

// file is an open non-directory. It honors the FOPEN flags as the
// kernel does: FOPEN_NONSEEKABLE and FOPEN_STREAM files cannot seek or
// read at an offset, a FOPEN_STREAM file reads at offset 0, and a
// FOPEN_NOFLUSH file is not flushed on Close. The flags for the page
// cache do not apply, as there is no cache.
type file struct {
	fsys *FS
	node *Node
	h    *Handle
	name string

	mu     sync.Mutex
	off    int64
	closed bool
}

var (
	_ io.ReaderAt = (*file)(nil)
	_ io.Seeker   = (*file)(nil)
)

func (f *file) Stat() (fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, &fs.PathError{Op: "stat", Path: f.name, Err: fs.ErrClosed}
	}
	out, errno := f.fsys.tree.Getattr(&f.fsys.caller, f.node.ID, f.h)
	if errno != 0 {
		return nil, pathError("stat", f.name, errno)
	}
	return &fileInfo{name: path.Base(f.name), attr: out.Attr}, nil
}

func (f *file) readAt(op string, p []byte, off int64) (int, error) {
	if f.closed {
		return 0, &fs.PathError{Op: op, Path: f.name, Err: fs.ErrClosed}
	}
	if off < 0 {
		return 0, &fs.PathError{Op: op, Path: f.name, Err: fs.ErrInvalid}
	}
	if len(p) == 0 {
		return 0, nil
	}
	n, errno := f.fsys.tree.Client.Read(&f.fsys.caller, f.h, uint64(off), p)
	if errno != 0 {
		return n, pathError(op, f.name, errno)
	}
	return n, nil
}

// seekable reports whether the file has offsets.
func (f *file) seekable() bool {
	return f.h.Flags()&(fuse.FOPEN_NONSEEKABLE|fuse.FOPEN_STREAM) == 0
}

func (f *file) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	off := f.off
	if f.h.Flags()&fuse.FOPEN_STREAM != 0 {
		// A stream has no position.
		off = 0
	}
	n, err := f.readAt("read", p, off)
	f.off += int64(n)
	if err == nil && n == 0 && len(p) > 0 {
		err = io.EOF
	}
	return n, err
}

func (f *file) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.seekable() {
		return 0, pathError("read", f.name, syscall.ESPIPE)
	}
	n, err := f.readAt("read", p, off)
	if err == nil && n < len(p) {
		err = io.EOF
	}
	return n, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, &fs.PathError{Op: "seek", Path: f.name, Err: fs.ErrClosed}
	}
	if !f.seekable() {
		return 0, pathError("seek", f.name, syscall.ESPIPE)
	}
	switch whence {
	case io.SeekCurrent:
		offset += f.off
	case io.SeekEnd:
		out, errno := f.fsys.tree.Getattr(&f.fsys.caller, f.node.ID, f.h)
		if errno != 0 {
			return 0, pathError("seek", f.name, errno)
		}
		offset += int64(out.Size)
	case io.SeekStart:
	default:
		return 0, &fs.PathError{Op: "seek", Path: f.name, Err: fs.ErrInvalid}
	}
	if offset < 0 {
		return 0, &fs.PathError{Op: "seek", Path: f.name, Err: fs.ErrInvalid}
	}
	f.off = offset
	return offset, nil
}

func (f *file) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return &fs.PathError{Op: "close", Path: f.name, Err: fs.ErrClosed}
	}
	f.closed = true
	var errno syscall.Errno
	if f.h.Flags()&fuse.FOPEN_NOFLUSH == 0 {
		errno = f.fsys.tree.Flush(&f.fsys.caller, f.h)
	}
	f.fsys.tree.Release(f.h)
	f.fsys.tree.Unref(f.node)
	if errno != 0 && errno != syscall.ENOSYS {
		return pathError("close", f.name, errno)
	}
	return nil
}

// dir is an open directory.
type dir struct {
	fsys *FS
	node *Node
	h    *Handle
	name string
	info *fileInfo

	mu      sync.Mutex
	off     uint64
	eof     bool
	pending []fs.DirEntry
	closed  bool
}

var _ fs.ReadDirFile = (*dir)(nil)

func (d *dir) Stat() (fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, &fs.PathError{Op: "stat", Path: d.name, Err: fs.ErrClosed}
	}
	out, errno := d.fsys.tree.Getattr(&d.fsys.caller, d.node.ID, nil)
	if errno != 0 {
		return nil, pathError("stat", d.name, errno)
	}
	return &fileInfo{name: d.info.name, attr: out.Attr}, nil
}

func (d *dir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: syscall.EISDIR}
}

// fill reads one batch of entries.
func (d *dir) fill() error {
	ents, errno := d.fsys.tree.Readdir(&d.fsys.caller, d.node, d.h, d.off, false)
	if errno != 0 {
		return pathError("readdir", d.name, errno)
	}
	if len(ents) == 0 {
		d.eof = true
		return nil
	}
	for _, e := range ents {
		d.off = e.Off
		if e.Name == "." || e.Name == ".." {
			continue
		}
		de := &dirEntry{fsys: d.fsys, path: path.Join(d.name, e.Name), name: e.Name, mode: e.Mode}
		if de.mode == 0 {
			// Type unknown: ask the file system.
			info, err := d.fsys.Lstat(de.path)
			if err != nil {
				return err
			}
			de.mode = info.Sys().(*fuse.Attr).Mode
		}
		d.pending = append(d.pending, de)
	}
	return nil
}

func (d *dir) ReadDir(count int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, &fs.PathError{Op: "readdir", Path: d.name, Err: fs.ErrClosed}
	}
	for !d.eof && (count <= 0 || len(d.pending) < count) {
		if err := d.fill(); err != nil {
			return nil, err
		}
	}
	n := len(d.pending)
	if count > 0 {
		if n == 0 {
			return nil, io.EOF
		}
		n = min(n, count)
	}
	res := d.pending[:n:n]
	d.pending = d.pending[n:]
	return res, nil
}

func (d *dir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return &fs.PathError{Op: "close", Path: d.name, Err: fs.ErrClosed}
	}
	d.closed = true
	d.fsys.tree.Release(d.h)
	d.fsys.tree.Unref(d.node)
	return nil
}

type dirEntry struct {
	fsys *FS
	path string
	name string
	mode uint32
}

func (e *dirEntry) Name() string               { return e.name }
func (e *dirEntry) IsDir() bool                { return e.mode&S_IFMT == fuse.S_IFDIR }
func (e *dirEntry) Type() fs.FileMode          { return fileMode(e.mode).Type() }
func (e *dirEntry) Info() (fs.FileInfo, error) { return e.fsys.Lstat(e.path) }

// fileInfo is an fs.FileInfo for FUSE attributes. Sys returns the
// *fuse.Attr.
type fileInfo struct {
	name string
	attr fuse.Attr
}

func (i *fileInfo) Name() string      { return i.name }
func (i *fileInfo) Size() int64       { return int64(i.attr.Size) }
func (i *fileInfo) Mode() fs.FileMode { return fileMode(i.attr.Mode) }
func (i *fileInfo) IsDir() bool       { return i.attr.Mode&S_IFMT == fuse.S_IFDIR }
func (i *fileInfo) Sys() any          { return &i.attr }
func (i *fileInfo) ModTime() time.Time {
	return time.Unix(int64(i.attr.Mtime), int64(i.attr.Mtimensec))
}

// fileMode converts a mode in the FUSE protocol (Linux values) to an
// fs.FileMode.
func fileMode(mode uint32) fs.FileMode {
	m := fs.FileMode(mode & 0777)
	switch mode & S_IFMT {
	case 0040000:
		m |= fs.ModeDir
	case 0100000:
	case 0120000:
		m |= fs.ModeSymlink
	case 0010000:
		m |= fs.ModeNamedPipe
	case 0140000:
		m |= fs.ModeSocket
	case 0020000:
		m |= fs.ModeDevice | fs.ModeCharDevice
	case 0060000:
		m |= fs.ModeDevice
	default:
		m |= fs.ModeIrregular
	}
	if mode&04000 != 0 {
		m |= fs.ModeSetuid
	}
	if mode&02000 != 0 {
		m |= fs.ModeSetgid
	}
	if mode&01000 != 0 {
		m |= fs.ModeSticky
	}
	return m
}
