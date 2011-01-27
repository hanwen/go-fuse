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
}

func NewPassThroughFuse(root string) (out *PassThroughFuse) {
	out = new(PassThroughFuse)
	out.root = root

	return out
}

func (self *PassThroughFuse) Mount(conn *fuse.PathFileSystemConnector) (fuse.Status) {
	return fuse.OK
}

func (self *PassThroughFuse) Unmount() {

}

func (self *PassThroughFuse) GetPath(relPath string) string {
	return path.Join(self.root, relPath)
}

func (self *PassThroughFuse) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	fullPath := self.GetPath(name)
	fi, err := os.Lstat(fullPath)
	if err != nil {
		return nil, fuse.ENOENT
	}
	out := new(fuse.Attr)
	fuse.CopyFileInfo(fi, out)

	return out, fuse.OK
}

func (self *PassThroughFuse) OpenDir(name string) (stream chan fuse.DirEntry, status fuse.Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(self.GetPath(name), os.O_RDONLY, 0)
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
		close(output)
		f.Close()
	}()	
	
	return output, fuse.OK
}

func (self *PassThroughFuse) Open(name string, flags uint32) (fuseFile fuse.RawFuseFile, status fuse.Status) {
	f, err := os.Open(self.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, fuse.OsErrorToFuseError(err)
	}
	return &PassThroughFile{file: f}, fuse.OK
}

func (self *PassThroughFuse) Chmod(path string, mode uint32) (code fuse.Status) {
	err := os.Chmod(self.GetPath(path), mode)
	return fuse.OsErrorToFuseError(err)
}

func (self *PassThroughFuse) Chown(path string, uid uint32, gid uint32) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Chown(self.GetPath(path), int(uid), int(gid)))
}

func (self *PassThroughFuse) Truncate(path string, offset uint64) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Truncate(self.GetPath(path), int64(offset)))
}

func (self *PassThroughFuse) Utimens(path string, AtimeNs uint64, MtimeNs uint64) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Chtimes(self.GetPath(path), int64(AtimeNs), int64(MtimeNs)))
}

func (self *PassThroughFuse) Readlink(name string) (out string, code fuse.Status) {
	f, err := os.Readlink(self.GetPath(name))
	return f, fuse.OsErrorToFuseError(err)
}

func (self *PassThroughFuse) Mknod(name string, mode uint32, dev uint32) (code fuse.Status) {
	return fuse.Status(syscall.Mknod(self.GetPath(name), mode, int(dev)))
}

func (self *PassThroughFuse) Mkdir(path string, mode uint32) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Mkdir(self.GetPath(path), mode))
}

func (self *PassThroughFuse) Unlink(name string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Remove(self.GetPath(name)))
}

func (self *PassThroughFuse) Rmdir(name string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Remove(self.GetPath(name)))
}

func (self *PassThroughFuse) Symlink(pointedTo string, linkName string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Symlink(pointedTo, self.GetPath(linkName)))
}

func (self *PassThroughFuse) Rename(oldPath string, newPath string) (code fuse.Status) {
	err := os.Rename(self.GetPath(oldPath), self.GetPath(newPath))
	return fuse.OsErrorToFuseError(err)
}

func (self *PassThroughFuse) Link(orig string, newName string) (code fuse.Status) {
	return fuse.OsErrorToFuseError(os.Link(self.GetPath(orig), self.GetPath(newName)))
}

func (self *PassThroughFuse) Access(name string, mode uint32) (code fuse.Status) {
	return fuse.Status(syscall.Access(self.GetPath(name), mode))
}

func (self *PassThroughFuse) Create(path string, flags uint32, mode uint32) (fuseFile fuse.RawFuseFile, code fuse.Status) {
	f, err := os.Open(self.GetPath(path), int(flags)|os.O_CREAT, mode)
	return &PassThroughFile{file: f}, fuse.OsErrorToFuseError(err)
}

func (self *PassThroughFuse) SetOptions(options *fuse.PathFileSystemConnectorOptions) {
	options.NegativeTimeout = 100.0
	options.AttrTimeout = 100.0
	options.EntryTimeout = 100.0
}

////////////////////////////////////////////////////////////////

type PassThroughFile struct {
	file *os.File
}

func (self *PassThroughFile) Read(input *fuse.ReadIn, buffers *fuse.BufferPool) ([]byte, fuse.Status) {
	slice := buffers.GetBuffer(input.Size)

	n, err := self.file.ReadAt(slice, int64(input.Offset))
	if err == os.EOF {
		// TODO - how to signal EOF?
		return slice[:n], fuse.OK
	}
	return slice[:n], fuse.OsErrorToFuseError(err)
}

func (self *PassThroughFile) Write(input *fuse.WriteIn, data []byte) (uint32, fuse.Status) {
	n, err := self.file.WriteAt(data, int64(input.Offset))
	return uint32(n), fuse.OsErrorToFuseError(err)
}

func (self *PassThroughFile) Flush() fuse.Status {
	return fuse.OK
}

func (self *PassThroughFile) Release() {
	self.file.Close()
}

func (self *PassThroughFile) Fsync(*fuse.FsyncIn) (code fuse.Status) {
	return fuse.Status(syscall.Fsync(self.file.Fd()))
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
	self := new(PassThroughDir)
	self.directoryChannel = make(chan *os.FileInfo, 500)
	go func() {
		for {
			want := 500
			infos, err := file.Readdir(want)
			for i, _ := range infos {
				self.directoryChannel <- &infos[i]
			}
			if len(infos) < want {
				break
			}
			if err != nil {
				self.directoryError = err
				break
			}
		}
		close(self.directoryChannel)
		file.Close()
	}()
	return self
}
