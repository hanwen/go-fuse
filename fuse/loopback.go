// A FUSE filesystem that shunts all request to an underlying file
// system.  Its main purpose is to provide test coverage without
// having to build an actual synthetic filesystem.

package fuse

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

var _ = fmt.Println
var _ = log.Println

type LoopbackFileSystem struct {
	Root string

	DefaultFileSystem
}

func NewLoopbackFileSystem(root string) (out *LoopbackFileSystem) {
	out = new(LoopbackFileSystem)
	out.Root = root

	return out
}

func (me *LoopbackFileSystem) GetPath(relPath string) string {
	return filepath.Join(me.Root, relPath)
}

func (me *LoopbackFileSystem) GetAttr(name string, context *Context) (a *Attr, code Status) {
	fullPath := me.GetPath(name)
	var err error = nil
	st := syscall.Stat_t{}
	if name == "" {
		// When GetAttr is called for the toplevel directory, we always want
		// to look through symlinks.
		err = syscall.Stat(fullPath, &st)
	} else {
		err = syscall.Lstat(fullPath, &st)
	}
	if err != nil {
		return nil, ToStatus(err)
	}
	a = &Attr{}
	a.FromStat(&st)
	return a, OK
}

func (me *LoopbackFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(me.GetPath(name))
	if err != nil {
		return nil, ToStatus(err)
	}
	want := 500
	output := make(chan DirEntry, want)
	go func() {
		for {
			infos, err := f.Readdir(want)
			for i := range infos {
				n := infos[i].Name()
				d := DirEntry{
					Name: n,
				}
				if s := ToStatT(infos[i]); s != nil {
					d.Mode = s.Mode
				} else {
					log.Println("ReadDir entry %q for %q has no stat info", n, name)
				}
				output <- d
			}
			if len(infos) < want || err == io.EOF {
				break
			}
			if err != nil {
				log.Println("Readdir() returned err:", err)
				break
			}
		}
		close(output)
		f.Close()
	}()

	return output, OK
}

func (me *LoopbackFileSystem) Open(name string, flags uint32, context *Context) (fuseFile File, status Status) {
	f, err := os.OpenFile(me.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, ToStatus(err)
	}
	return &LoopbackFile{File: f}, OK
}

func (me *LoopbackFileSystem) Chmod(path string, mode uint32, context *Context) (code Status) {
	err := os.Chmod(me.GetPath(path), os.FileMode(mode))
	return ToStatus(err)
}

func (me *LoopbackFileSystem) Chown(path string, uid uint32, gid uint32, context *Context) (code Status) {
	return ToStatus(os.Chown(me.GetPath(path), int(uid), int(gid)))
}

func (me *LoopbackFileSystem) Truncate(path string, offset uint64, context *Context) (code Status) {
	return ToStatus(os.Truncate(me.GetPath(path), int64(offset)))
}

func (me *LoopbackFileSystem) Utimens(path string, AtimeNs int64, MtimeNs int64, context *Context) (code Status) {
	return ToStatus(os.Chtimes(me.GetPath(path), time.Unix(0, AtimeNs), time.Unix(0, MtimeNs)))
}

func (me *LoopbackFileSystem) Readlink(name string, context *Context) (out string, code Status) {
	f, err := os.Readlink(me.GetPath(name))
	return f, ToStatus(err)
}

func (me *LoopbackFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) (code Status) {
	return ToStatus(syscall.Mknod(me.GetPath(name), mode, int(dev)))
}

func (me *LoopbackFileSystem) Mkdir(path string, mode uint32, context *Context) (code Status) {
	return ToStatus(os.Mkdir(me.GetPath(path), os.FileMode(mode)))
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (me *LoopbackFileSystem) Unlink(name string, context *Context) (code Status) {
	return ToStatus(syscall.Unlink(me.GetPath(name)))
}

func (me *LoopbackFileSystem) Rmdir(name string, context *Context) (code Status) {
	return ToStatus(syscall.Rmdir(me.GetPath(name)))
}

func (me *LoopbackFileSystem) Symlink(pointedTo string, linkName string, context *Context) (code Status) {
	return ToStatus(os.Symlink(pointedTo, me.GetPath(linkName)))
}

func (me *LoopbackFileSystem) Rename(oldPath string, newPath string, context *Context) (code Status) {
	err := os.Rename(me.GetPath(oldPath), me.GetPath(newPath))
	return ToStatus(err)
}

func (me *LoopbackFileSystem) Link(orig string, newName string, context *Context) (code Status) {
	return ToStatus(os.Link(me.GetPath(orig), me.GetPath(newName)))
}

func (me *LoopbackFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return ToStatus(syscall.Access(me.GetPath(name), mode))
}

func (me *LoopbackFileSystem) Create(path string, flags uint32, mode uint32, context *Context) (fuseFile File, code Status) {
	f, err := os.OpenFile(me.GetPath(path), int(flags)|os.O_CREATE, os.FileMode(mode))
	return &LoopbackFile{File: f}, ToStatus(err)
}

func (me *LoopbackFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	data := make([]byte, 1024)
	data, errNo := GetXAttr(me.GetPath(name), attr, data)

	return data, Status(errNo)
}

func (me *LoopbackFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	data, errNo := ListXAttr(me.GetPath(name))

	return data, Status(errNo)
}

func (me *LoopbackFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return Status(Removexattr(me.GetPath(name), attr))
}

func (me *LoopbackFileSystem) String() string {
	return fmt.Sprintf("LoopbackFileSystem(%s)", me.Root)
}

func (me *LoopbackFileSystem) StatFs(name string) *StatfsOut {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(me.GetPath(name), &s)
	if err == nil {
		return &StatfsOut{
			raw.Kstatfs{
				Blocks:  s.Blocks,
				Bsize:   uint32(s.Bsize),
				Bfree:   s.Bfree,
				Bavail:  s.Bavail,
				Files:   s.Files,
				Ffree:   s.Ffree,
				Frsize:  uint32(s.Frsize),
				NameLen: uint32(s.Namelen),
			},
		}
	}
	return nil
}
