// A FUSE filesystem that shunts all request to an underlying file
// system.  Its main purpose is to provide test coverage without
// having to build an actual synthetic filesystem.

package fuse

import (
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

func CopyFileInfo(fi *os.FileInfo, attr *Attr) {
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

func (self *PassThroughFuse) Init() (*InitOut, Status) {
	return new(InitOut), OK
}

func (self *PassThroughFuse) Destroy() {

}

func (self *PassThroughFuse) GetPath(relPath string) string {
	return path.Join(self.root, relPath)
}

func (self *PassThroughFuse) GetAttr(name string) (*Attr, Status) {
	fullPath := self.GetPath(name)
	fi, err := os.Lstat(fullPath)
	if err != nil {
		return nil, ENOENT
	}
	out := new(Attr)
	CopyFileInfo(fi, out)

	return out, OK
}

func (self *PassThroughFuse) OpenDir(name string) (fuseFile RawFuseDir, status Status) {
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(self.GetPath(name), os.O_RDONLY, 0)
	if err != nil {
		return nil, OsErrorToFuseError(err)
	}
	p := NewPassThroughDir(f)
	return p, OK
}

func (self *PassThroughFuse) Open(name string, flags uint32) (fuseFile RawFuseFile, status Status) {
	f, err := os.Open(self.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, OsErrorToFuseError(err)
	}
	return &PassThroughFile{file: f}, OK
}

func (self *PassThroughFuse) Chmod(path string, mode uint32) (code Status) {
	err := os.Chmod(self.GetPath(path), mode)
	return OsErrorToFuseError(err)
}

func (self *PassThroughFuse) Chown(path string, uid uint32, gid uint32) (code Status) {
	return OsErrorToFuseError(os.Chown(self.GetPath(path), int(uid), int(gid)))
}

func (self *PassThroughFuse) Truncate(path string, offset uint64) (code Status) {
	return OsErrorToFuseError(os.Truncate(self.GetPath(path), int64(offset)))
}

func (self *PassThroughFuse) Utimens(path string, AtimeNs uint64, MtimeNs uint64) (code Status) {
	return OsErrorToFuseError(os.Chtimes(self.GetPath(path), int64(AtimeNs), int64(MtimeNs)))
}

func (self *PassThroughFuse) Readlink(name string) (out string, code Status) {
	f, err := os.Readlink(self.GetPath(name))
	return f, OsErrorToFuseError(err)
}

func (self *PassThroughFuse) Mknod(name string, mode uint32, dev uint32) (code Status) {
	return Status(syscall.Mknod(self.GetPath(name), mode, int(dev)))
}

func (self *PassThroughFuse) Mkdir(path string, mode uint32) (code Status) {
	return OsErrorToFuseError(os.Mkdir(self.GetPath(path), mode))
}

func (self *PassThroughFuse) Unlink(name string) (code Status) {
	return OsErrorToFuseError(os.Remove(self.GetPath(name)))
}

func (self *PassThroughFuse) Rmdir(name string) (code Status) {
	return OsErrorToFuseError(os.Remove(self.GetPath(name)))
}

func (self *PassThroughFuse) Symlink(pointedTo string, linkName string) (code Status) {
	return OsErrorToFuseError(os.Symlink(pointedTo, self.GetPath(linkName)))
}

func (self *PassThroughFuse) Rename(oldPath string, newPath string) (code Status) {
	err := os.Rename(self.GetPath(oldPath), self.GetPath(newPath))
	return OsErrorToFuseError(err)
}

func (self *PassThroughFuse) Link(orig string, newName string) (code Status) {
	return OsErrorToFuseError(os.Link(self.GetPath(orig), self.GetPath(newName)))
}

func (self *PassThroughFuse) Access(name string, mode uint32) (code Status) {
	return Status(syscall.Access(self.GetPath(name), mode))
}

func (self *PassThroughFuse) Create(path string, flags uint32, mode uint32) (fuseFile RawFuseFile, code Status) {
	f, err := os.Open(self.GetPath(path), int(flags)|os.O_CREAT, mode)
	return &PassThroughFile{file: f}, OsErrorToFuseError(err)
}

////////////////////////////////////////////////////////////////

type PassThroughFile struct {
	file *os.File
}

func (self *PassThroughFile) Read(input *ReadIn) ([]byte, Status) {
	buf := make([]byte, input.Size)
	slice := buf[:]

	n, err := self.file.ReadAt(slice, int64(input.Offset))
	if err == os.EOF {
		// TODO - how to signal EOF?
		return slice[:n], OK
	}
	return slice[:n], OsErrorToFuseError(err)
}

func (self *PassThroughFile) Write(input *WriteIn, data []byte) (uint32, Status) {
	n, err := self.file.WriteAt(data, int64(input.Offset))
	return uint32(n), OsErrorToFuseError(err)
}

func (self *PassThroughFile) Flush() Status {
	return OK
}

func (self *PassThroughFile) Release() {
	self.file.Close()
}

func (self *PassThroughFile) Fsync(*FsyncIn) (code Status) {
	return Status(syscall.Fsync(self.file.Fd()))
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

func (self *PassThroughDir) ReadDir(input *ReadIn) (*DirEntryList, Status) {
	list := NewDirEntryList(int(input.Size))

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
	return list, OsErrorToFuseError(self.directoryError)
}

func (self *PassThroughDir) ReleaseDir() {
	close(self.directoryChannel)
}

func (self *PassThroughDir) FsyncDir(input *FsyncIn) (code Status) {
	return ENOSYS
}
