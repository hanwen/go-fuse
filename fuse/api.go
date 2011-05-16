// The fuse package provides APIs to implement filesystems in
// userspace, using libfuse on Linux.
package fuse

import (
	"os"
)

// Types for users to implement.


// A filesystem API that uses paths rather than inodes.  A minimal
// file system should have at least a functional GetAttr method.
// Typically, each call happens in its own goroutine, so take care to
// make the file system thread-safe.
//
// Include DefaultFileSystem to provide a default null implementation of
// required methods.
type FileSystem interface {
	// Attributes
	GetAttr(name string) (*os.FileInfo, Status)

	// These should update the file's ctime too.
	Chmod(name string, mode uint32) (code Status)
	Chown(name string, uid uint32, gid uint32) (code Status)
	Utimens(name string, AtimeNs uint64, MtimeNs uint64) (code Status)

	Truncate(name string, offset uint64) (code Status)

	Access(name string, mode uint32) (code Status)

	// Tree structure
	Link(oldName string, newName string) (code Status)
	Mkdir(name string, mode uint32) Status
	Mknod(name string, mode uint32, dev uint32) Status
	Rename(oldName string, newName string) (code Status)
	Rmdir(name string) (code Status)
	Unlink(name string) (code Status)

	// Extended attributes.
	GetXAttr(name string, attribute string) (data []byte, code Status)
	ListXAttr(name string) (attributes []string, code Status)
	RemoveXAttr(name string, attr string) Status
	SetXAttr(name string, attr string, data []byte, flags int) Status

	// Called after mount.
	Mount(connector *FileSystemConnector) Status
	Unmount()

	// File handling.  If opening for writing, the file's mtime
	// should be updated too.
	Open(name string, flags uint32) (file File, code Status)
	Create(name string, flags uint32, mode uint32) (file File, code Status)

	// Flush() gets called as a file opened for read/write.
	Flush(name string) Status

	// Directory handling
	OpenDir(name string) (stream chan DirEntry, code Status)

	// Symlinks.
	Symlink(value string, linkName string) (code Status)
	Readlink(name string) (string, Status)
}

// A File object should be returned from FileSystem.Open and
// FileSystem.Create.  Include DefaultFile into the struct to inherit
// a default null implementation.
//
// TODO - should File be thread safe?
type File interface {
	Read(*ReadIn, BufferPool) ([]byte, Status)
	Write(*WriteIn, []byte) (written uint32, code Status)
	Truncate(size uint64) Status

	GetAttr() (*os.FileInfo, Status)
	Chown(uid uint32, gid uint32) Status
	Chmod(perms uint32) Status
	Utimens(atimeNs uint64, mtimeNs uint64) Status
	Flush() Status
	Release()
	Fsync(*FsyncIn) (code Status)
	Ioctl(input *IoctlIn) (output *IoctlOut, data []byte, code Status)
}

// MountOptions contains time out options for a FileSystem.  The
// default copied from libfuse and set in NewMountOptions() is
// (1s,1s,0s).
type FileSystemOptions struct {
	EntryTimeout    float64
	AttrTimeout     float64
	NegativeTimeout float64
}

type MountOptions struct {
	AllowOther      bool
}

// DefaultFileSystem implements a FileSystem that returns ENOSYS for every operation.
type DefaultFileSystem struct{}

// DefaultFile returns ENOSYS for every operation.
type DefaultFile struct{}

// RawFileSystem is an interface closer to the FUSE wire protocol. 
//
// Unless you really know what you are doing, you should not implement
// this, but rather the FileSystem interface; the details of getting
// interactions with open files, renames, and threading right etc. are
// somewhat tricky and not very interesting.
//
// Include DefaultRawFileSystem to inherit a null implementation.
type RawFileSystem interface {
	Destroy(h *InHeader, input *InitIn)
	Lookup(header *InHeader, name string) (out *EntryOut, status Status)
	Forget(header *InHeader, input *ForgetIn)

	// Attributes.
	GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status)
	SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status)

	// Modifying structure.
	Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status)
	Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status)
	Unlink(header *InHeader, name string) (code Status)
	Rmdir(header *InHeader, name string) (code Status)
	Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status)
	Link(header *InHeader, input *LinkIn, filename string) (out *EntryOut, code Status)

	Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status)
	Readlink(header *InHeader) (out []byte, code Status)
	Access(header *InHeader, input *AccessIn) (code Status)

	// Extended attributes.
	GetXAttr(header *InHeader, attr string) (data []byte, code Status)
	ListXAttr(header *InHeader) (attributes []byte, code Status)
	SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status
	RemoveXAttr(header *InHeader, attr string) (code Status)

	// File handling.
	Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status)
	Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status)
	Read(*ReadIn, BufferPool) ([]byte, Status)

	Release(header *InHeader, input *ReleaseIn)
	Write(*WriteIn, []byte) (written uint32, code Status)
	Flush(*FlushIn) Status
	Fsync(*FsyncIn) (code Status)

	// Directory handling
	OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status)
	ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status)
	ReleaseDir(header *InHeader, input *ReleaseIn)
	FsyncDir(header *InHeader, input *FsyncIn) (code Status)

	//
	Ioctl(header *InHeader, input *IoctlIn) (output *IoctlOut, data []byte, code Status)
}

// DefaultRawFileSystem returns ENOSYS for every operation.
type DefaultRawFileSystem struct{}
