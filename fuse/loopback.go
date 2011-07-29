// A FUSE filesystem that shunts all request to an underlying file
// system.  Its main purpose is to provide test coverage without
// having to build an actual synthetic filesystem.

package fuse

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
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

func (me *LoopbackFileSystem) GetAttr(name string) (*os.FileInfo, Status) {
	fullPath := me.GetPath(name)
	fi, err := os.Lstat(fullPath)
	if err != nil {
		return nil, OsErrorToErrno(err)
	}
	return fi, OK
}

func (me *LoopbackFileSystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(me.GetPath(name))
	if err != nil {
		return nil, OsErrorToErrno(err)
	}
	want := 500
	output := make(chan DirEntry, want)
	go func() {
		for {
			infos, err := f.Readdir(want)
			for i, _ := range infos {
				output <- DirEntry{
					Name: infos[i].Name,
					Mode: infos[i].Mode,
				}
			}
			if len(infos) < want || err == os.EOF {
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

func (me *LoopbackFileSystem) Open(name string, flags uint32) (fuseFile File, status Status) {
	f, err := os.OpenFile(me.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, OsErrorToErrno(err)
	}
	return &LoopbackFile{File: f}, OK
}

func (me *LoopbackFileSystem) Chmod(path string, mode uint32) (code Status) {
	err := os.Chmod(me.GetPath(path), mode)
	return OsErrorToErrno(err)
}

func (me *LoopbackFileSystem) Chown(path string, uid uint32, gid uint32) (code Status) {
	return OsErrorToErrno(os.Chown(me.GetPath(path), int(uid), int(gid)))
}

func (me *LoopbackFileSystem) Truncate(path string, offset uint64) (code Status) {
	return OsErrorToErrno(os.Truncate(me.GetPath(path), int64(offset)))
}

func (me *LoopbackFileSystem) Utimens(path string, AtimeNs uint64, MtimeNs uint64) (code Status) {
	return OsErrorToErrno(os.Chtimes(me.GetPath(path), int64(AtimeNs), int64(MtimeNs)))
}

func (me *LoopbackFileSystem) Readlink(name string) (out string, code Status) {
	f, err := os.Readlink(me.GetPath(name))
	return f, OsErrorToErrno(err)
}

func (me *LoopbackFileSystem) Mknod(name string, mode uint32, dev uint32) (code Status) {
	return Status(syscall.Mknod(me.GetPath(name), mode, int(dev)))
}

func (me *LoopbackFileSystem) Mkdir(path string, mode uint32) (code Status) {
	return OsErrorToErrno(os.Mkdir(me.GetPath(path), mode))
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (me *LoopbackFileSystem) Unlink(name string) (code Status) {
	return Status(syscall.Unlink(me.GetPath(name)))
}

func (me *LoopbackFileSystem) Rmdir(name string) (code Status) {
	return Status(syscall.Rmdir(me.GetPath(name)))
}

func (me *LoopbackFileSystem) Symlink(pointedTo string, linkName string) (code Status) {
	return OsErrorToErrno(os.Symlink(pointedTo, me.GetPath(linkName)))
}

func (me *LoopbackFileSystem) Rename(oldPath string, newPath string) (code Status) {
	err := os.Rename(me.GetPath(oldPath), me.GetPath(newPath))
	return OsErrorToErrno(err)
}

func (me *LoopbackFileSystem) Link(orig string, newName string) (code Status) {
	return OsErrorToErrno(os.Link(me.GetPath(orig), me.GetPath(newName)))
}

func (me *LoopbackFileSystem) Access(name string, mode uint32) (code Status) {
	return Status(syscall.Access(me.GetPath(name), mode))
}

func (me *LoopbackFileSystem) Create(path string, flags uint32, mode uint32) (fuseFile File, code Status) {
	f, err := os.OpenFile(me.GetPath(path), int(flags)|os.O_CREATE, mode)
	return &LoopbackFile{File: f}, OsErrorToErrno(err)
}

func (me *LoopbackFileSystem) GetXAttr(name string, attr string) ([]byte, Status) {
	data, errNo := GetXAttr(me.GetPath(name), attr)

	return data, Status(errNo)
}

func (me *LoopbackFileSystem) ListXAttr(name string) ([]string, Status) {
	data, errNo := ListXAttr(me.GetPath(name))

	return data, Status(errNo)
}

func (me *LoopbackFileSystem) RemoveXAttr(name string, attr string) Status {
	return Status(Removexattr(me.GetPath(name), attr))
}

func (me *LoopbackFileSystem) Name() string {
	return fmt.Sprintf("LoopbackFileSystem(%s)", me.Root)
}

