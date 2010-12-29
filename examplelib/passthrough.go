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

func CopyFileInfo(fi *os.FileInfo, attr *fuse.Attr) {
	attr.Ino = uint64(fi.Ino)
	attr.Size = uint64(fi.Size)
	attr.Blocks = uint64(fi.Blocks)

	attr.Atime = uint64(fi.Atime_ns / 1e9)
	attr.Atimensec = uint32(fi.Atime_ns % 1e9)

	attr.Mtime = uint64(fi.Mtime_ns / 1e9)
	attr.Mtimensec = uint32(fi.Mtime_ns % 1e9)

	attr.Ctime = uint64(fi.Ctime_ns / 1e9)
	attr.Ctimensec = uint32(fi.Ctime_ns % 1e9)

	attr.Mode = fi.Mode
	attr.Nlink = uint32(fi.Nlink)
	attr.Uid = uint32(fi.Uid)
	attr.Gid = uint32(fi.Gid)
	attr.Rdev = uint32(fi.Rdev)
	attr.Blksize = uint32(fi.Blksize)
}

func (self *PassThroughFuse) Init() (*fuse.InitOut, fuse.Status) {
	return new(fuse.InitOut), fuse.OK
}

func (self *PassThroughFuse) Destroy() {

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
	CopyFileInfo(fi, out)

	return out, fuse.OK
}

func (self *PassThroughFuse) OpenDir(name string) (fuseFile fuse.RawFuseDir, status fuse.Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(self.GetPath(name), os.O_RDONLY, 0)
	if err != nil {
		return nil, fuse.OsErrorToFuseError(err)
	}
	p := NewPassThroughDir(f)
	return p, fuse.OK
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

////////////////////////////////////////////////////////////////

type PassThroughFile struct {
	file *os.File
}

func (self *PassThroughFile) Read(input *fuse.ReadIn) ([]byte, fuse.Status) {
	buf := make([]byte, input.Size)
	slice := buf[:]

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
	directoryError os.Error
	shipped int 
	exported int
	leftOver *os.FileInfo
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

func (self *PassThroughDir) ReadDir(input *fuse.ReadIn) (*fuse.DirEntryList, fuse.Status) {
	list := fuse.NewDirEntryList(int(input.Size))

	if self.leftOver != nil {
		success := list.AddString(self.leftOver.Name, self.leftOver.Ino, self.leftOver.Mode)
		self.exported++
		if !success {
			panic("No space for single entry.")
		}
		self.leftOver = nil
	}
	
	for {
		fi := <-self.directoryChannel
		if fi == nil {
			break
		}
		if !list.AddString(fi.Name, fi.Ino, fi.Mode) {
			self.leftOver = fi
			break
		} 
	}
	return list, fuse.OsErrorToFuseError(self.directoryError)
}

func (self *PassThroughDir) ReleaseDir() {
	close(self.directoryChannel)
}

func (self *PassThroughDir) FsyncDir(input *fuse.FsyncIn) (code fuse.Status) {
	return fuse.ENOSYS
}
