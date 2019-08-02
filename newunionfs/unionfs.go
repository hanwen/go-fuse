// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unionfs

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func filePathHash(path string) string {
	dir, base := filepath.Split(path)

	h := md5.New()
	h.Write([]byte(dir))
	return fmt.Sprintf("%x-%s", h.Sum(nil)[:8], base)
}

type unionFSRoot struct {
	unionFSNode

	roots []string
}

type unionFSNode struct {
	fs.Inode
}

const delDir = "DELETIONS"

var delDirHash = filePathHash(delDir)

func (r *unionFSRoot) allMarkers(result map[string]struct{}) syscall.Errno {
	dir := filepath.Join(r.roots[0], delDir)

	ds, errno := fs.NewLoopbackDirStream(dir)
	if errno != 0 {
		return errno
	}

	defer ds.Close()
	for ds.HasNext() {
		e, errno := ds.Next()
		if errno != 0 {
			return errno
		}
		if e.Mode != syscall.S_IFREG {
			continue
		}
		result[e.Name] = struct{}{}
	}

	return 0
}

func (r *unionFSRoot) rmMarker(name string) syscall.Errno {
	err := syscall.Unlink(r.markerPath(name))
	if err != nil {
		return err.(syscall.Errno)
	}
	return 0
}

func (r *unionFSRoot) writeMarker(name string) syscall.Errno {
	dir := filepath.Join(r.roots[0], delDir)
	var st syscall.Stat_t
	if err := syscall.Stat(dir, &st); err == syscall.ENOENT {
		if err := syscall.Mkdir(dir, 0755); err != nil {
			log.Printf("Mkdir %q: %v", dir, err)
			return syscall.EIO
		}
	} else if err != nil {
		return err.(syscall.Errno)
	}

	dest := r.markerPath(name)

	err := ioutil.WriteFile(dest, []byte(name), 0644)
	return fs.ToErrno(err)
}

func (r *unionFSRoot) markerPath(name string) string {
	return filepath.Join(r.roots[0], delDir, filePathHash(name))
}

func (r *unionFSRoot) isDeleted(name string) bool {
	var st syscall.Stat_t
	err := syscall.Stat(r.markerPath(name), &st)
	return err == nil
}

func (n *unionFSNode) root() *unionFSRoot {
	return n.Root().Operations().(*unionFSRoot)
}

var _ = (fs.NodeSetattrer)((*unionFSNode)(nil))

func (n *unionFSNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if errno := n.promote(); errno != 0 {
		return errno
	}

	if fh != nil {
		return fh.(fs.FileSetattrer).Setattr(ctx, in, out)
	}

	p := filepath.Join(n.root().roots[0], n.Path(nil))
	fsa, ok := fh.(fs.FileSetattrer)
	if ok && fsa != nil {
		fsa.Setattr(ctx, in, out)
	} else {
		if m, ok := in.GetMode(); ok {
			if err := syscall.Chmod(p, m); err != nil {
				return fs.ToErrno(err)
			}
		}

		uid, uok := in.GetUID()
		gid, gok := in.GetGID()
		if uok || gok {
			suid := -1
			sgid := -1
			if uok {
				suid = int(uid)
			}
			if gok {
				sgid = int(gid)
			}
			if err := syscall.Chown(p, suid, sgid); err != nil {
				return fs.ToErrno(err)
			}
		}

		mtime, mok := in.GetMTime()
		atime, aok := in.GetATime()

		if mok || aok {

			ap := &atime
			mp := &mtime
			if !aok {
				ap = nil
			}
			if !mok {
				mp = nil
			}
			var ts [2]syscall.Timespec
			ts[0] = fuse.UtimeToTimespec(ap)
			ts[1] = fuse.UtimeToTimespec(mp)

			if err := syscall.UtimesNano(p, ts[:]); err != nil {
				return fs.ToErrno(err)
			}
		}

		if sz, ok := in.GetSize(); ok {
			if err := syscall.Truncate(p, int64(sz)); err != nil {
				return fs.ToErrno(err)
			}
		}
	}

	fga, ok := fh.(fs.FileGetattrer)
	if ok && fga != nil {
		fga.Getattr(ctx, out)
	} else {
		st := syscall.Stat_t{}
		err := syscall.Lstat(p, &st)
		if err != nil {
			return fs.ToErrno(err)
		}
		out.FromStat(&st)
	}
	return 0
}

var _ = (fs.NodeCreater)((*unionFSNode)(nil))

func (n *unionFSNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	if n.IsRoot() && name == delDir {
		return nil, nil, 0, syscall.EPERM
	}

	var st syscall.Stat_t
	dirName, idx := n.getBranch(&st)
	if idx > 0 {
		if errno := n.promote(); errno != 0 {
			return nil, nil, 0, errno
		}
		idx = 0
	}
	fullPath := filepath.Join(dirName, name)
	r := n.root()
	if errno := r.rmMarker(fullPath); errno != 0 && errno != syscall.ENOENT {
		return nil, nil, 0, errno
	}

	abs := filepath.Join(n.root().roots[0], fullPath)
	fd, err := syscall.Creat(abs, mode)
	if err != nil {
		return nil, nil, 0, err.(syscall.Errno)
	}

	if err := syscall.Fstat(fd, &st); err != nil {
		// now what?
		syscall.Close(fd)
		syscall.Unlink(abs)
		return nil, nil, 0, err.(syscall.Errno)
	}

	ch := n.NewInode(ctx, &unionFSNode{}, fs.StableAttr{Mode: st.Mode, Ino: st.Ino})
	out.FromStat(&st)

	return ch, fs.NewLoopbackFile(fd), 0, 0
}

var _ = (fs.NodeOpener)((*unionFSNode)(nil))

func (n *unionFSNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	isWR := (flags&syscall.O_RDWR != 0) || (flags&syscall.O_WRONLY != 0)

	var st syscall.Stat_t
	nm, idx := n.getBranch(&st)
	if isWR && idx > 0 {
		if errno := n.promote(); errno != 0 {
			return nil, 0, errno
		}
		idx = 0
	}

	fd, err := syscall.Open(filepath.Join(n.root().roots[idx], nm), int(flags), 0)
	if err != nil {
		return nil, 0, err.(syscall.Errno)
	}

	return fs.NewLoopbackFile(fd), 0, 0
}

var _ = (fs.NodeGetattrer)((*unionFSNode)(nil))

func (n *unionFSNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	var st syscall.Stat_t
	_, idx := n.getBranch(&st)
	if idx < 0 {
		return syscall.ENOENT
	}

	out.FromStat(&st)
	return 0
}

var _ = (fs.NodeLookuper)((*unionFSNode)(nil))

func (n *unionFSNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if n.IsRoot() && name == delDir {
		return nil, syscall.ENOENT
	}

	var st syscall.Stat_t

	p := filepath.Join(n.Path(nil), name)
	idx := n.root().getBranch(p, &st)
	if idx >= 0 {
		// XXX use idx in Ino?
		ch := n.NewInode(ctx, &unionFSNode{}, fs.StableAttr{Mode: st.Mode, Ino: st.Ino})
		out.FromStat(&st)
		out.Mode |= 0111
		return ch, 0
	}
	return nil, syscall.ENOENT
}

var _ = (fs.NodeUnlinker)((*unionFSNode)(nil))

func (n *unionFSNode) Unlink(ctx context.Context, name string) syscall.Errno {
	return n.root().delPath(filepath.Join(n.Path(nil), name))
}

var _ = (fs.NodeRmdirer)((*unionFSNode)(nil))

func (n *unionFSNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	return n.root().delPath(filepath.Join(n.Path(nil), name))
}

var _ = (fs.NodeSymlinker)((*unionFSNode)(nil))

func (n *unionFSNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.promote()
	path := filepath.Join(n.root().roots[0], n.Path(nil), name)
	err := syscall.Symlink(target, path)

	if err != nil {
		return nil, err.(syscall.Errno)
	}

	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return nil, err.(syscall.Errno)
	}

	out.FromStat(&st)

	ch := n.NewInode(ctx, &unionFSNode{}, fs.StableAttr{
		Mode: syscall.S_IFLNK,
		Ino:  st.Ino,
	})
	return ch, 0
}

var _ = (fs.NodeReadlinker)((*unionFSNode)(nil))

func (n *unionFSNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	nm, idx := n.getBranch(nil)

	var buf [1024]byte
	count, err := syscall.Readlink(filepath.Join(n.root().roots[idx], nm), buf[:])
	if err != nil {
		return nil, err.(syscall.Errno)
	}

	return buf[:count], 0
}

var _ = (fs.NodeReaddirer)((*unionFSNode)(nil))

func (n *unionFSNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	root := n.root()

	markers := map[string]struct{}{delDirHash: struct{}{}}
	// ignore error: assume no markers
	root.allMarkers(markers)

	dir := n.Path(nil)

	names := map[string]uint32{}
	for i := range root.roots {
		// deepest root first.
		readRoot(root.roots[len(root.roots)-i-1], dir, names)
	}
	result := make([]fuse.DirEntry, 0, len(names))
	for nm, mode := range names {
		marker := filePathHash(filepath.Join(dir, nm))
		if _, ok := markers[marker]; ok {
			continue
		}
		result = append(result, fuse.DirEntry{
			Name: nm,
			Mode: mode,
		})
	}

	return fs.NewListDirStream(result), 0
}

func readRoot(root string, dir string, result map[string]uint32) {
	ds, errno := fs.NewLoopbackDirStream(filepath.Join(root, dir))
	if errno != 0 {
		return
	}
	defer ds.Close()
	for ds.HasNext() {
		e, errno := ds.Next()
		if errno != 0 {
			return
		}

		result[e.Name] = e.Mode
	}
}

// getBranch returns the root where we can find the given file. It
// will check the deletion markers in roots[0].
func (n *unionFSNode) getBranch(st *syscall.Stat_t) (string, int) {
	name := n.Path(nil)
	return name, n.root().getBranch(name, st)
}

func (r *unionFSRoot) getBranch(name string, st *syscall.Stat_t) int {
	if r.isDeleted(name) {
		return -1
	}
	if st == nil {
		st = &syscall.Stat_t{}
	}
	for i, root := range r.roots {
		p := filepath.Join(root, name)
		err := syscall.Lstat(p, st)
		if err == nil {
			return i
		}
	}
	return -1
}

func (n *unionFSRoot) delPath(p string) syscall.Errno {
	var st syscall.Stat_t
	r := n.root()
	idx := r.getBranch(p, &st)

	if idx < 0 {
		return 0
	}
	if idx == 0 {
		err := syscall.Unlink(filepath.Join(r.roots[idx], p))
		if err != nil {
			return fs.ToErrno(err)
		}
		idx = r.getBranch(p, &st)
	}
	if idx > 0 {
		return r.writeMarker(p)
	}

	return 0
}

func (n *unionFSNode) promote() syscall.Errno {
	p := &n.Inode
	r := n.root()

	type tup struct {
		*unionFSNode
		name string
		idx  int
		st   syscall.Stat_t
	}

	var parents []tup
	for p != nil && p != &r.Inode {
		asUN := p.Operations().(*unionFSNode)

		var st syscall.Stat_t
		name, idx := asUN.getBranch(&st)
		if idx == 0 {
			break
		}
		if idx < 0 {
			log.Println("promote called on nonexistent file")
			return syscall.EIO
		}

		parents = append(parents, tup{asUN, name, idx, st})
		_, p = p.Parent()
	}

	for i := len(parents) - 1; i >= 0; i-- {
		t := parents[i]

		path := t.Path(nil)
		if t.IsDir() {
			if err := syscall.Mkdir(filepath.Join(r.roots[0], path), t.st.Mode); err != nil {
				return err.(syscall.Errno)
			}
		} else if t.Mode()&syscall.S_IFREG != 0 {
			if errno := r.promoteRegularFile(path, t.idx, &t.st); errno != 0 {
				return errno
			}
		} else {
			log.Panicf("don't know how to handle mode %o", t.Mode())
		}
		var ts [2]syscall.Timespec
		ts[0] = t.st.Atim
		ts[1] = t.st.Mtim

		// ignore error.
		syscall.UtimesNano(path, ts[:])
	}
	return 0
}

func (r *unionFSRoot) promoteRegularFile(p string, idx int, st *syscall.Stat_t) syscall.Errno {
	dest, err := syscall.Creat(filepath.Join(r.roots[0], p), st.Mode)
	if err != nil {
		return err.(syscall.Errno)
	}
	src, err := syscall.Open(filepath.Join(r.roots[idx], p), syscall.O_RDONLY, 0)
	if err != nil {
		syscall.Close(dest)
		return err.(syscall.Errno)
	}

	var ret syscall.Errno
	var buf [128 >> 10]byte
	for {
		n, err := syscall.Read(src, buf[:])
		if n == 0 {
			break
		}
		if err != nil {
			ret = err.(syscall.Errno)
			break
		}

		if _, err := syscall.Write(dest, buf[:n]); err != nil {
			ret = err.(syscall.Errno)
			break
		}
	}
	syscall.Close(src)

	if err := syscall.Close(dest); err != nil && ret == 0 {
		ret = err.(syscall.Errno)
	}
	return ret
}
