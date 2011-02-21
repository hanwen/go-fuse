// A FUSE filesystem that shunts all request to an underlying file
// system.  Its main purpose is to provide test coverage without
// having to build an actual synthetic filesystem.

package examplelib

import (
	"github.com/hanwen/go-fuse/fuse"
	"fmt"
	"os"
	"path"
	"syscall"
)

var _ = fmt.Println

type PassThroughFuse struct {
	root string

	fuse.DefaultPathFilesystem
}

func NewPassThroughFuse(root string) (out *PassThroughFuse) {
	out = new(PassThroughFuse)
	out.root = root

	return out
}

func (me *PassThroughFuse) GetPath(relPath string) string {
	return path.Join(me.root, relPath)
}

func (me *PassThroughFuse) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	fullPath := me.GetPath(name)
	fi, err := os.Lstat(fullPath)
	if err != nil {
		return nil, fuse.ENOENT
	}
	out := new(fuse.Attr)
	fuse.CopyFileInfo(fi, out)

	return out, fuse.OK
}

func (me *PassThroughFuse) OpenDir(name string) (stream chan fuse.DirEntry, status fuse.Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(me.GetPath(name), os.O_RDONLY, 0)
	if err != nil {
		return nil, fuse.OsErrorToFuseError(err)
	}
	output := make(chan fuse.DirEntry, 500)
	go func() {
		for {
			want := 500
			infos, err := f.Readdir(want)
			for i, _ := range infos {
				output <- fuse.DirEntry{
					Name: infos[i].Name,
					Mode: infos[i].Mode,
				}
			}
			if len(infos) < want {
				break
			}
			if err != nil {
				// TODO - how to signal error
				break
			}
		}
		output <- fuse.DirEntry{}
		f.Close()
	}()

	return output, fuse.OK
}

func (me *PassThroughFuse) Open(name string, flags uint32) (fuseFile fuse.RawFuseFile, status fuse.Status) {
	f, err := os.Open(me.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, fuse.OsErrorToFuseError(err)
	}
	return &PassThroughFile{file: f}, fuse.OK
}

func (me *PassThroughFuse) Chmod(path string, mode uint32) (code fuse.Status) {
	err := os.Chmod(me.GetPath(path), mode)
	return fuse.OsErrorToFuseError(err)
}

func (me *PassThroughFuse) Chown(path string, uid uint32, gid uint32) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Chown(me.GetPath(path), int(uid), int(gid)))
}

func (me *PassThroughFuse) Truncate(path string, offset uint64) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Truncate(me.GetPath(path), int64(offset)))
}

func (me *PassThroughFuse) Utimens(path string, AtimeNs uint64, MtimeNs uint64) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Chtimes(me.GetPath(path), int64(AtimeNs), int64(MtimeNs)))
}

func (me *PassThroughFuse) Readlink(name string) (out string, code fuse.Status) {
	f, err := os.Readlink(me.GetPath(name))
	return f, fuse.OsErrorToFuseError(err)
}

func (me *PassThroughFuse) Mknod(name string, mode uint32, dev uint32) (code fuse.Status) {
	return fuse.Status(syscall.Mknod(me.GetPath(name), mode, int(dev)))
}

func (me *PassThroughFuse) Mkdir(path string, mode uint32) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Mkdir(me.GetPath(path), mode))
}

func (me *PassThroughFuse) Unlink(name string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Remove(me.GetPath(name)))
}

func (me *PassThroughFuse) Rmdir(name string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Remove(me.GetPath(name)))
}

func (me *PassThroughFuse) Symlink(pointedTo string, linkName string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Symlink(pointedTo, me.GetPath(linkName)))
}

func (me *PassThroughFuse) Rename(oldPath string, newPath string) (code fuse.Status) {
	err := os.Rename(me.GetPath(oldPath), me.GetPath(newPath))
	return fuse.OsErrorToFuseError(err)
}

func (me *PassThroughFuse) Link(orig string, newName string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Link(me.GetPath(orig), me.GetPath(newName)))
}

func (me *PassThroughFuse) Access(name string, mode uint32) (code fuse.Status) {
	return fuse.Status(syscall.Access(me.GetPath(name), mode))
}

func (me *PassThroughFuse) Create(path string, flags uint32, mode uint32) (fuseFile fuse.RawFuseFile, code fuse.Status) {
	f, err := os.Open(me.GetPath(path), int(flags)|os.O_CREAT, mode)
	return &PassThroughFile{file: f}, fuse.OsErrorToFuseError(err)
}

func (me *PassThroughFuse) SetOptions(options *fuse.PathFileSystemConnectorOptions) {
	options.NegativeTimeout = 100.0
	options.AttrTimeout = 100.0
	options.EntryTimeout = 100.0
}

////////////////////////////////////////////////////////////////

type PassThroughFile struct {
	file *os.File

	fuse.DefaultRawFuseFile
}

func (me *PassThroughFile) Read(input *fuse.ReadIn, buffers *fuse.BufferPool) ([]byte, fuse.Status) {
	slice := buffers.AllocBuffer(input.Size)

	n, err := me.file.ReadAt(slice, int64(input.Offset))
	if err == os.EOF {
		// TODO - how to signal EOF?
		return slice[:n], fuse.OK
	}
	return slice[:n], fuse.OsErrorToFuseError(err)
}

func (me *PassThroughFile) Write(input *fuse.WriteIn, data []byte) (uint32, fuse.Status) {
	n, err := me.file.WriteAt(data, int64(input.Offset))
	return uint32(n), fuse.OsErrorToFuseError(err)
}

func (me *PassThroughFile) Release() {
	me.file.Close()
}

func (me *PassThroughFile) Fsync(*fuse.FsyncIn) (code fuse.Status) {
	return fuse.Status(syscall.Fsync(me.file.Fd()))
}

////////////////////////////////////////////////////////////////

type PassThroughDir struct {
	directoryChannel chan *os.FileInfo
	directoryError   os.Error
	shipped          int
	exported         int
	leftOver         *os.FileInfo
}

func NewPassThroughDir(file *os.File) *PassThroughDir {
	me := new(PassThroughDir)
	me.directoryChannel = make(chan *os.FileInfo, 500)
	go func() {
		for {
			want := 500
			infos, err := file.Readdir(want)
			for i, _ := range infos {
				me.directoryChannel <- &infos[i]
			}
			if len(infos) < want {
				break
			}
			if err != nil {
				me.directoryError = err
				break
			}
		}
		close(me.directoryChannel)
		file.Close()
	}()
	return me
}
