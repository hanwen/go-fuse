package fuse

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var _ = fmt.Println
var _ = log.Println

// A FUSE filesystem that shunts all request to an underlying file
// system.  Its main purpose is to provide test coverage without
// having to build an actual synthetic filesystem.
type LoopbackFileSystem struct {
	Root string
	DefaultFileSystem
}

func NewLoopbackFileSystem(root string) (out *LoopbackFileSystem) {
	out = new(LoopbackFileSystem)
	out.Root = root

	return out
}

func (fs *LoopbackFileSystem) GetPath(relPath string) string {
	return filepath.Join(fs.Root, relPath)
}

func (fs *LoopbackFileSystem) GetAttr(name string, context *Context) (a *Attr, code Status) {
	fullPath := fs.GetPath(name)
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

var _ = (FileSystem)((*LoopbackFileSystem)(nil))

func (fs *LoopbackFileSystem) OpenDir(name string, context *Context) (stream []DirEntry, status Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(fs.GetPath(name))
	if err != nil {
		return nil, ToStatus(err)
	}
	want := 500
	output := make([]DirEntry, 0, want)
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
				log.Printf("ReadDir entry %q for %q has no stat info", n, name)
			}
			output = append(output, d)
		}
		if len(infos) < want || err == io.EOF {
			break
		}
		if err != nil {
			log.Println("Readdir() returned err:", err)
			break
		}
	}
	f.Close()

	return output, OK
}

func (fs *LoopbackFileSystem) Open(name string, flags uint32, context *Context) (fuseFile File, status Status) {
	f, err := os.OpenFile(fs.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, ToStatus(err)
	}
	return &LoopbackFile{File: f}, OK
}

func (fs *LoopbackFileSystem) Chmod(path string, mode uint32, context *Context) (code Status) {
	err := os.Chmod(fs.GetPath(path), os.FileMode(mode))
	return ToStatus(err)
}

func (fs *LoopbackFileSystem) Chown(path string, uid uint32, gid uint32, context *Context) (code Status) {
	return ToStatus(os.Chown(fs.GetPath(path), int(uid), int(gid)))
}

func (fs *LoopbackFileSystem) Truncate(path string, offset uint64, context *Context) (code Status) {
	return ToStatus(os.Truncate(fs.GetPath(path), int64(offset)))
}

func (fs *LoopbackFileSystem) Utimens(path string, AtimeNs int64, MtimeNs int64, context *Context) (code Status) {
	return ToStatus(os.Chtimes(fs.GetPath(path), time.Unix(0, AtimeNs), time.Unix(0, MtimeNs)))
}

func (fs *LoopbackFileSystem) Readlink(name string, context *Context) (out string, code Status) {
	f, err := os.Readlink(fs.GetPath(name))
	return f, ToStatus(err)
}

func (fs *LoopbackFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) (code Status) {
	return ToStatus(syscall.Mknod(fs.GetPath(name), mode, int(dev)))
}

func (fs *LoopbackFileSystem) Mkdir(path string, mode uint32, context *Context) (code Status) {
	return ToStatus(os.Mkdir(fs.GetPath(path), os.FileMode(mode)))
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (fs *LoopbackFileSystem) Unlink(name string, context *Context) (code Status) {
	return ToStatus(syscall.Unlink(fs.GetPath(name)))
}

func (fs *LoopbackFileSystem) Rmdir(name string, context *Context) (code Status) {
	return ToStatus(syscall.Rmdir(fs.GetPath(name)))
}

func (fs *LoopbackFileSystem) Symlink(pointedTo string, linkName string, context *Context) (code Status) {
	return ToStatus(os.Symlink(pointedTo, fs.GetPath(linkName)))
}

func (fs *LoopbackFileSystem) Rename(oldPath string, newPath string, context *Context) (code Status) {
	err := os.Rename(fs.GetPath(oldPath), fs.GetPath(newPath))
	return ToStatus(err)
}

func (fs *LoopbackFileSystem) Link(orig string, newName string, context *Context) (code Status) {
	return ToStatus(os.Link(fs.GetPath(orig), fs.GetPath(newName)))
}

func (fs *LoopbackFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return ToStatus(syscall.Access(fs.GetPath(name), mode))
}

func (fs *LoopbackFileSystem) Create(path string, flags uint32, mode uint32, context *Context) (fuseFile File, code Status) {
	f, err := os.OpenFile(fs.GetPath(path), int(flags)|os.O_CREATE, os.FileMode(mode))
	return &LoopbackFile{File: f}, ToStatus(err)
}

func (fs *LoopbackFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	data := make([]byte, 1024)
	data, errNo := GetXAttr(fs.GetPath(name), attr, data)

	return data, Status(errNo)
}

func (fs *LoopbackFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	data, errNo := ListXAttr(fs.GetPath(name))

	return data, Status(errNo)
}

func (fs *LoopbackFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return Status(Removexattr(fs.GetPath(name), attr))
}

func (fs *LoopbackFileSystem) String() string {
	return fmt.Sprintf("LoopbackFs(%s)", fs.Root)
}

func (fs *LoopbackFileSystem) StatFs(name string) *StatfsOut {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.GetPath(name), &s)
	if err == nil {
		return &StatfsOut{
			Blocks:  s.Blocks,
			Bsize:   uint32(s.Bsize),
			Bfree:   s.Bfree,
			Bavail:  s.Bavail,
			Files:   s.Files,
			Ffree:   s.Ffree,
			Frsize:  uint32(s.Frsize),
			NameLen: uint32(s.Namelen),
		}
	}
	return nil
}
