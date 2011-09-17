package fuse

import (
	"path/filepath"
	"os"
	"sort"
	"strings"
	"syscall"
)

// SwitchFileSystem construct the union of a set of filesystems, and
// select them by prefix.  Consider the traditional Unix file system:
// different parts have different characteristics, eg. "/usr" is
// read-only, while "/home/user" is read/write.  Similarly, "/tmp"
// does not need to be persistent and "/dev" is totally unlike a file
// system.
//
// With SwitchFileSystem, you can write filesystems for each of these
// parts separately, and combine them using SwitchFileSystem.  This is
// a simpler and less efficient alternative to in-process mounts, but
// it also works if the encompassing file system already has the
// mounted directory.
type SwitchFileSystem struct {
	DefaultFileSystem
	fileSystems SwitchedFileSystems
}

// This is the definition of one member of SwitchedFileSystems.
type SwitchedFileSystem struct {
	Prefix string
	FileSystem
	StripPrefix bool
}

type SwitchedFileSystems []*SwitchedFileSystem

func (p SwitchedFileSystems) Len() int {
	return len(p)
}

func (p SwitchedFileSystems) Less(i, j int) bool {
	// Invert order, so we get more specific prefixes first.
	return p[i].Prefix > p[j].Prefix
}

func (p SwitchedFileSystems) Swap(i, j int) {
	swFs := p[i]
	p[i] = p[j]
	p[j] = swFs
}

func NewSwitchFileSystem(fsMap []SwitchedFileSystem) *SwitchFileSystem {
	me := &SwitchFileSystem{}
	for _, inSwFs := range fsMap {
		swFs := inSwFs
		swFs.Prefix = strings.TrimLeft(swFs.Prefix, string(filepath.Separator))
		swFs.Prefix = strings.TrimRight(swFs.Prefix, string(filepath.Separator))
		me.fileSystems = append(me.fileSystems, &swFs)
	}
	sort.Sort(me.fileSystems)
	return me
}

// TODO - use binary search.  This is inefficient if there are large
// numbers of switched filesystems.
func (me *SwitchFileSystem) findFileSystem(path string) (string, *SwitchedFileSystem) {
	for _, swFs := range me.fileSystems {
		if swFs.Prefix == "" || swFs.Prefix == path || strings.HasPrefix(path, swFs.Prefix+string(filepath.Separator)) {
			if swFs.StripPrefix {
				path = strings.TrimLeft(path[len(swFs.Prefix):], string(filepath.Separator))
			}

			return path, swFs
		}
	}
	return "", nil
}

func (me *SwitchFileSystem) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return nil, ENOENT
	}
	return fs.FileSystem.GetAttr(name, context)
}

func (me *SwitchFileSystem) Readlink(name string, context *Context) (string, Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return "", ENOENT
	}
	return fs.FileSystem.Readlink(name, context)
}

func (me *SwitchFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Mknod(name, mode, dev, context)
}

func (me *SwitchFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Mkdir(name, mode, context)
}

func (me *SwitchFileSystem) Unlink(name string, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Unlink(name, context)
}

func (me *SwitchFileSystem) Rmdir(name string, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Rmdir(name, context)
}

func (me *SwitchFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	linkName, fs := me.findFileSystem(linkName)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Symlink(value, linkName, context)
}

func (me *SwitchFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	oldName, fs1 := me.findFileSystem(oldName)
	newName, fs2 := me.findFileSystem(newName)
	if fs1 != fs2 {
		return syscall.EXDEV
	}
	if fs1 == nil {
		return ENOENT
	}
	return fs1.Rename(oldName, newName, context)
}

func (me *SwitchFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	oldName, fs1 := me.findFileSystem(oldName)
	newName, fs2 := me.findFileSystem(newName)
	if fs1 != fs2 {
		return syscall.EXDEV
	}
	if fs1 == nil {
		return ENOENT
	}
	return fs1.Link(oldName, newName, context)
}

func (me *SwitchFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Chmod(name, mode, context)
}

func (me *SwitchFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Chown(name, uid, gid, context)
}

func (me *SwitchFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Truncate(name, offset, context)
}

func (me *SwitchFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return nil, ENOENT
	}
	return fs.FileSystem.Open(name, flags, context)
}

func (me *SwitchFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return nil, ENOENT
	}
	return fs.FileSystem.OpenDir(name, context)
}

func (me *SwitchFileSystem) OnMount(nodeFs *PathNodeFs) {
	for _, fs := range me.fileSystems {
		fs.FileSystem.OnMount(nodeFs)
	}
}

func (me *SwitchFileSystem) OnUnmount() {
	for _, fs := range me.fileSystems {
		fs.FileSystem.OnUnmount()
	}
}

func (me *SwitchFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Access(name, mode, context)
}

func (me *SwitchFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return nil, ENOENT
	}
	return fs.FileSystem.Create(name, flags, mode, context)
}

func (me *SwitchFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64, context *Context) (code Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.Utimens(name, AtimeNs, CtimeNs, context)
}

func (me *SwitchFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return nil, ENOENT
	}
	return fs.FileSystem.GetXAttr(name, attr, context)
}

func (me *SwitchFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.SetXAttr(name, attr, data, flags, context)
}

func (me *SwitchFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return nil, ENOENT
	}
	return fs.FileSystem.ListXAttr(name, context)
}

func (me *SwitchFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	name, fs := me.findFileSystem(name)
	if fs == nil {
		return ENOENT
	}
	return fs.FileSystem.RemoveXAttr(name, attr, context)
}
