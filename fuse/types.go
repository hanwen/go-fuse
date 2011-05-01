package fuse

import (
	"os"
	"syscall"
)

const (
	FUSE_ROOT_ID = 1

	FUSE_UNKNOWN_INO = 0xffffffff

	CUSE_UNRESTRICTED_IOCTL = (1 << 0)

	FUSE_RELEASE_FLUSH = (1 << 0)

	FUSE_LK_FLOCK = (1 << 0)

	FUSE_IOCTL_MAX_IOV = 256

	FUSE_POLL_SCHEDULE_NOTIFY = (1 << 0)

	CUSE_INIT_INFO_MAX = 4096

	S_IFDIR = syscall.S_IFDIR
	S_IFREG = syscall.S_IFREG

	// TODO - get this from a canonical place.
	PAGESIZE = 4096

	CUSE_INIT = 4096

	O_ANYWRITE = uint32(os.O_WRONLY | os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_TRUNC)
)

const (
	// TODO - we should read this from /sys/fs/fuse/ , dynamically.
	_BACKGROUND_TASKS = 12
)

type Status int32

const (
	OK      = Status(0)
	EACCES  = Status(syscall.EACCES)
	EBUSY   = Status(syscall.EBUSY)
	EINVAL  = Status(syscall.EINVAL)
	EIO     = Status(syscall.EIO)
	ENOENT  = Status(syscall.ENOENT)
	ENOSYS  = Status(syscall.ENOSYS)
	ENOTDIR = Status(syscall.ENOTDIR)
	EPERM   = Status(syscall.EPERM)
	ERANGE  = Status(syscall.ERANGE)
	EXDEV   = Status(syscall.EXDEV)
)


type NotifyCode int

const (
	FUSE_NOTIFY_POLL        = 1
	FUSE_NOTIFY_INVAL_INODE = 2
	FUSE_NOTIFY_INVAL_ENTRY = 3
	FUSE_NOTIFY_CODE_MAX    = 4
)

type Attr struct {
	Ino       uint64
	Size      uint64
	Blocks    uint64
	Atime     uint64
	Mtime     uint64
	Ctime     uint64
	Atimensec uint32
	Mtimensec uint32
	Ctimensec uint32
	Mode      uint32
	Nlink     uint32
	Owner
	Rdev    uint32
	Blksize uint32
	Padding uint32
}

type Owner struct {
	Uid uint32
	Gid uint32
}

type Identity struct {
	Owner
	Pid uint32
}

type Kstatfs struct {
	Blocks  uint64
	Bfree   uint64
	Bavail  uint64
	Files   uint64
	Ffree   uint64
	Bsize   uint32
	NameLen uint32
	Frsize  uint32
	Padding uint32
	Spare   [6]uint32
}

type FileLock struct {
	Start uint64
	End   uint64
	Typ   uint32
	Pid   uint32
}

type EntryOut struct {
	NodeId         uint64
	Generation     uint64
	EntryValid     uint64
	AttrValid      uint64
	EntryValidNsec uint32
	AttrValidNsec  uint32
	Attr
}

type ForgetIn struct {
	Nlookup uint64
}

const (
	// Mask for GetAttrIn.Flags. If set, GetAttrIn has a file handle set.
	FUSE_GETATTR_FH = (1 << 0)
)

type GetAttrIn struct {
	Flags uint32
	Dummy uint32
	Fh    uint64
}

type AttrOut struct {
	AttrValid     uint64
	AttrValidNsec uint32
	Dummy         uint32
	Attr
}

type MknodIn struct {
	Mode    uint32
	Rdev    uint32
	Umask   uint32
	Padding uint32
}

type MkdirIn struct {
	Mode  uint32
	Umask uint32
}

type RenameIn struct {
	Newdir uint64
}

type LinkIn struct {
	Oldnodeid uint64
}


const ( // SetAttrIn.Valid
	FATTR_MODE      = (1 << 0)
	FATTR_UID       = (1 << 1)
	FATTR_GID       = (1 << 2)
	FATTR_SIZE      = (1 << 3)
	FATTR_ATIME     = (1 << 4)
	FATTR_MTIME     = (1 << 5)
	FATTR_FH        = (1 << 6)
	FATTR_ATIME_NOW = (1 << 7)
	FATTR_MTIME_NOW = (1 << 8)
	FATTR_LOCKOWNER = (1 << 9)
)

type SetAttrIn struct {
	Valid     uint32
	Padding   uint32
	Fh        uint64
	Size      uint64
	LockOwner uint64
	Atime     uint64
	Mtime     uint64
	Unused2   uint64
	Atimensec uint32
	Mtimensec uint32
	Unused3   uint32
	Mode      uint32
	Unused4   uint32
	Owner
	Unused5 uint32
}

const (
	// OpenIn.Flags
	FOPEN_DIRECT_IO   = (1 << 0)
	FOPEN_KEEP_CACHE  = (1 << 1)
	FOPEN_NONSEEKABLE = (1 << 2)
)

type OpenIn struct {
	Flags  uint32
	Unused uint32
}

type CreateIn struct {
	Flags   uint32
	Mode    uint32
	Umask   uint32
	Padding uint32
}

type OpenOut struct {
	Fh        uint64
	OpenFlags uint32
	Padding   uint32
}

type CreateOut struct {
	EntryOut
	OpenOut
}

type ReleaseIn struct {
	Fh           uint64
	Flags        uint32
	ReleaseFlags uint32
	LockOwner    uint64
}

type FlushIn struct {
	Fh        uint64
	Unused    uint32
	Padding   uint32
	LockOwner uint64
}

const (
	FUSE_READ_LOCKOWNER = (1 << 1)
)

type ReadIn struct {
	Fh        uint64
	Offset    uint64
	Size      uint32
	ReadFlags uint32
	LockOwner uint64
	Flags     uint32
	Padding   uint32
}


const (
	FUSE_WRITE_CACHE     = (1 << 0)
	FUSE_WRITE_LOCKOWNER = (1 << 1)
)

type WriteIn struct {
	Fh         uint64
	Offset     uint64
	Size       uint32
	WriteFlags uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}

type WriteOut struct {
	Size    uint32
	Padding uint32
}

type StatfsOut struct {
	Kstatfs
}

type FsyncIn struct {
	Fh         uint64
	FsyncFlags uint32
	Padding    uint32
}

type SetXAttrIn struct {
	Size  uint32
	Flags uint32
}

type GetXAttrIn struct {
	Size    uint32
	Padding uint32
}

type GetXAttrOut struct {
	Size    uint32
	Padding uint32
}

type LkIn struct {
	Fh      uint64
	Owner   uint64
	Lk      FileLock
	LkFlags uint32
	Padding uint32
}

type LkOut struct {
	Lk FileLock
}

type AccessIn struct {
	Mask    uint32
	Padding uint32
}

// To be set in InitIn/InitOut.Flags.
const (
	CAP_ASYNC_READ     = (1 << 0)
	CAP_POSIX_LOCKS    = (1 << 1)
	CAP_FILE_OPS       = (1 << 2)
	CAP_ATOMIC_O_TRUNC = (1 << 3)
	CAP_EXPORT_SUPPORT = (1 << 4)
	CAP_BIG_WRITES     = (1 << 5)
	CAP_DONT_MASK      = (1 << 6)
	CAP_SPLICE_WRITE   = (1 << 7)
	CAP_SPLICE_MOVE    = (1 << 8)
	CAP_SPLICE_READ    = (1 << 9)
)

type InitIn struct {
	Major        uint32
	Minor        uint32
	MaxReadAhead uint32
	Flags        uint32
}

type InitOut struct {
	Major               uint32
	Minor               uint32
	MaxReadAhead        uint32
	Flags               uint32
	MaxBackground       uint16
	CongestionThreshold uint16
	MaxWrite            uint32
}

type CuseInitIn struct {
	Major  uint32
	Minor  uint32
	Unused uint32
	Flags  uint32
}

type CuseInitOut struct {
	Major    uint32
	Minor    uint32
	Unused   uint32
	Flags    uint32
	MaxRead  uint32
	MaxWrite uint32
	DevMajor uint32
	DevMinor uint32
	Spare    [10]uint32
}

type InterruptIn struct {
	Unique uint64
}

type BmapIn struct {
	Block     uint64
	Blocksize uint32
	Padding   uint32
}

type BmapOut struct {
	Block uint64
}

const (
	FUSE_IOCTL_COMPAT       = (1 << 0)
	FUSE_IOCTL_UNRESTRICTED = (1 << 1)
	FUSE_IOCTL_RETRY        = (1 << 2)
)

type IoctlIn struct {
	Fh      uint64
	Flags   uint32
	Cmd     uint32
	Arg     uint64
	InSize  uint32
	OutSize uint32
}

type IoctlOut struct {
	Result  int32
	Flags   uint32
	InIovs  uint32
	OutIovs uint32
}

type PollIn struct {
	Fh      uint64
	Kh      uint64
	Flags   uint32
	Padding uint32
}

type PollOut struct {
	Revents uint32
	Padding uint32
}

type NotifyPollWakeupOut struct {
	Kh uint64
}

type InHeader struct {
	Length uint32
	opcode
	Unique uint64
	NodeId uint64
	Identity
	Padding uint32
}

type OutHeader struct {
	Length uint32
	Status Status
	Unique uint64
}

type Dirent struct {
	Ino     uint64
	Off     uint64
	NameLen uint32
	Typ     uint32
}

type NotifyInvalInodeOut struct {
	Ino    uint64
	Off    int64
	Length int64
}

type NotifyInvalEntryOut struct {
	Parent  uint64
	NameLen uint32
	Padding uint32
}


////////////////////////////////////////////////////////////////
// Types for users to implement.

// This is the interface to the file system, mirroring the interface from
//
//   /usr/include/fuse/fuse_lowlevel.h
//
// Typically, each call happens in its own goroutine, so any global
// data should be made thread-safe.  Unless you really know what you
// are doing, you should not implement this, but FileSystem below;
// the details of getting interactions with open files, renames, and
// threading right etc. are somewhat tricky and not very interesting.
type RawFileSystem interface {
	Destroy(h *InHeader, input *InitIn)
	Lookup(header *InHeader, name string) (out *EntryOut, status Status)
	Forget(header *InHeader, input *ForgetIn)

	GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status)
	SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status)

	Readlink(header *InHeader) (out []byte, code Status)
	Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status)
	Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status)
	Unlink(header *InHeader, name string) (code Status)
	Rmdir(header *InHeader, name string) (code Status)

	Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status)

	Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status)
	Link(header *InHeader, input *LinkIn, filename string) (out *EntryOut, code Status)

	GetXAttr(header *InHeader, attr string) (data []byte, code Status)
	ListXAttr(header *InHeader) (attributes []byte, code Status)
	SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status
	RemoveXAttr(header *InHeader, attr string) (code Status)
	Access(header *InHeader, input *AccessIn) (code Status)
	Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status)

	/*
		 	// unimplemented.
			Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status)
			Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status)
			Poll(header *InHeader, input *PollIn) (out *PollOut, code Status)
	*/

	// File handling.
	Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status)
	Read(*ReadIn, *BufferPool) ([]byte, Status)
	Release(header *InHeader, input *ReleaseIn)
	Write(*WriteIn, []byte) (written uint32, code Status)
	Flush(*FlushIn) Status
	Fsync(*FsyncIn) (code Status)

	// Directory handling
	OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status)
	ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status)
	ReleaseDir(header *InHeader, input *ReleaseIn)
	FsyncDir(header *InHeader, input *FsyncIn) (code Status)
}

type File interface {
	Read(*ReadIn, *BufferPool) ([]byte, Status)
	// u32 <-> u64 ?
	Write(*WriteIn, []byte) (written uint32, code Status)
	Flush() Status
	Release()
	Fsync(*FsyncIn) (code Status)

	GetAttr() *Attr
	Utimens(atimeNs uint64, mtimeNs uint64) Status
	Truncate(size uint64) Status
	Chown(uid uint32, gid uint32) Status
	Chmod(perms uint32) Status
}

type RawDir interface {
	ReadDir(input *ReadIn) (*DirEntryList, Status)
	Release()
}

type FileSystem interface {
	GetAttr(name string) (*Attr, Status)
	Readlink(name string) (string, Status)
	Mknod(name string, mode uint32, dev uint32) Status
	Mkdir(name string, mode uint32) Status
	Unlink(name string) (code Status)
	Rmdir(name string) (code Status)
	Symlink(value string, linkName string) (code Status)
	Rename(oldName string, newName string) (code Status)
	Link(oldName string, newName string) (code Status)
	Chmod(name string, mode uint32) (code Status)
	Chown(name string, uid uint32, gid uint32) (code Status)
	Truncate(name string, offset uint64) (code Status)
	Open(name string, flags uint32) (file File, code Status)

	GetXAttr(name string, attribute string) (data []byte, code Status)
	SetXAttr(name string, attr string, data []byte, flags int) Status
	ListXAttr(name string) (attributes []string, code Status)
	RemoveXAttr(name string, attr string) Status

	// Where to hook up statfs?

	OpenDir(name string) (stream chan DirEntry, code Status)

	// TODO - what is a good interface?
	Mount(connector *FileSystemConnector) Status
	Unmount()

	Access(name string, mode uint32) (code Status)
	Create(name string, flags uint32, mode uint32) (file File, code Status)
	Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status)

	// unimplemented: poll, ioctl, bmap.
}

// MountOptions contains time out options for a FileSystem.  The
// default copied from libfuse and set in NewMountOptions() is
// (1s,1s,0s).
type MountOptions struct {
	EntryTimeout    float64
	AttrTimeout     float64
	NegativeTimeout float64
}

// Include these structs in your implementation to inherit default nop
// implementations.

type DefaultFileSystem struct{}
type DefaultFile struct{}
type DefaultRawFileSystem struct{}
