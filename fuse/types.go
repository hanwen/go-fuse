package fuse

import (
	"syscall"
)

const (

	/** Version number of this interface */
	FUSE_KERNEL_VERSION = 7

	/** Minor version number of this interface */
	FUSE_KERNEL_MINOR_VERSION = 13

	/** The node ID of the root inode */
	FUSE_ROOT_ID = 1

	/**
	 * Bitmasks for SetattrIn.valid
	 */
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

	/**
	 * Flags returned by the OPEN request
	 *
	 * FOPEN_DIRECT_IO: bypass page cache for this open file
	 * FOPEN_KEEP_CACHE: don't invalidate the data cache on open
	 * FOPEN_NONSEEKABLE: the file is not seekable
	 */
	FOPEN_DIRECT_IO   = (1 << 0)
	FOPEN_KEEP_CACHE  = (1 << 1)
	FOPEN_NONSEEKABLE = (1 << 2)

	/**
	 * INIT request/reply flags
	 *
	 * FUSE_EXPORT_SUPPORT: filesystem handles lookups of "." and ".."
	 * FUSE_DONT_MASK: don't apply umask to file mode on create operations
	 */
	FUSE_ASYNC_READ     = (1 << 0)
	FUSE_POSIX_LOCKS    = (1 << 1)
	FUSE_FILE_OPS       = (1 << 2)
	FUSE_ATOMIC_O_TRUNC = (1 << 3)
	FUSE_EXPORT_SUPPORT = (1 << 4)
	FUSE_BIG_WRITES     = (1 << 5)
	FUSE_DONT_MASK      = (1 << 6)

	/**
	 * CUSE INIT request/reply flags
	 *
	 * CUSE_UNRESTRICTED_IOCTL:  use unrestricted ioctl
	 */
	CUSE_UNRESTRICTED_IOCTL = (1 << 0)

	/**
	 * Release flags
	 */
	FUSE_RELEASE_FLUSH = (1 << 0)

	/**
	 * GetAttr flags
	 */
	FUSE_GETATTR_FH = (1 << 0)

	/**
	 * Lock flags
	 */
	FUSE_LK_FLOCK = (1 << 0)

	/**
	 * WRITE flags
	 *
	 * FUSE_WRITE_CACHE: delayed write from page cache, file handle is guessed
	 * FUSE_WRITE_LOCKOWNER: lockOwner field is valid
	 */
	FUSE_WRITE_CACHE     = (1 << 0)
	FUSE_WRITE_LOCKOWNER = (1 << 1)

	/**
	 * Read flags
	 */
	FUSE_READ_LOCKOWNER = (1 << 1)

	/**
	 * Ioctl flags
	 *
	 * FUSE_IOCTL_COMPAT: 32bit compat ioctl on 64bit machine
	 * FUSE_IOCTL_UNRESTRICTED: not restricted to well-formed ioctls, retry allowed
	 * FUSE_IOCTL_RETRY: retry with new iovecs
	 *
	 * FUSE_IOCTL_MAX_IOV: maximum of in_iovecs + out_iovecs
	 */
	FUSE_IOCTL_COMPAT       = (1 << 0)
	FUSE_IOCTL_UNRESTRICTED = (1 << 1)
	FUSE_IOCTL_RETRY        = (1 << 2)

	FUSE_IOCTL_MAX_IOV = 256

	/**
	 * Poll flags
	 *
	 * FUSE_POLL_SCHEDULE_NOTIFY: request poll notify
	 */
	FUSE_POLL_SCHEDULE_NOTIFY = (1 << 0)

	FUSE_COMPAT_WRITE_IN_SIZE = 24

	/* The read buffer is required to be at least 8k, but may be much larger */
	FUSE_MIN_READ_BUFFER = 8192

	FUSE_COMPAT_ENTRY_OUT_SIZE = 120

	FUSE_COMPAT_ATTR_OUT_SIZE = 96

	FUSE_COMPAT_MKNOD_IN_SIZE = 8

	FUSE_COMPAT_STATFS_SIZE = 48

	CUSE_INIT_INFO_MAX = 4096

	S_IFDIR = syscall.S_IFDIR
)

type Error int32

const (
	OK      = Error(0)
	EIO     = Error(syscall.EIO)
	ENOSYS  = Error(syscall.ENOSYS)
	ENODATA = Error(syscall.ENODATA)
)

type Opcode int

const (
	FUSE_LOOKUP      = 1
	FUSE_FORGET      = 2 /* no reply */
	FUSE_GETATTR     = 3
	FUSE_SETATTR     = 4
	FUSE_READLINK    = 5
	FUSE_SYMLINK     = 6
	FUSE_MKNOD       = 8
	FUSE_MKDIR       = 9
	FUSE_UNLINK      = 10
	FUSE_RMDIR       = 11
	FUSE_RENAME      = 12
	FUSE_LINK        = 13
	FUSE_OPEN        = 14
	FUSE_READ        = 15
	FUSE_WRITE       = 16
	FUSE_STATFS      = 17
	FUSE_RELEASE     = 18
	FUSE_FSYNC       = 20
	FUSE_SETXATTR    = 21
	FUSE_GETXATTR    = 22
	FUSE_LISTXATTR   = 23
	FUSE_REMOVEXATTR = 24
	FUSE_FLUSH       = 25
	FUSE_INIT        = 26
	FUSE_OPENDIR     = 27
	FUSE_READDIR     = 28
	FUSE_RELEASEDIR  = 29
	FUSE_FSYNCDIR    = 30
	FUSE_GETLK       = 31
	FUSE_SETLK       = 32
	FUSE_SETLKW      = 33
	FUSE_ACCESS      = 34
	FUSE_CREATE      = 35
	FUSE_INTERRUPT   = 36
	FUSE_BMAP        = 37
	FUSE_DESTROY     = 38
	FUSE_IOCTL       = 39
	FUSE_POLL        = 40

	/* CUSE specific operations */
	CUSE_INIT = 4096
)

type NotifyCode int

const (
	FUSE_NOTIFY_POLL        = 1
	FUSE_NOTIFY_INVAL_INODE = 2
	FUSE_NOTIFY_INVAL_ENTRY = 3
	FUSE_NOTIFY_CODE_MAX    = 4
)

/* Make sure all structures are padded to 64bit boundary, so 32bit
   userspace works under 64bit kernels */

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
	Uid       uint32
	Gid       uint32
	Rdev      uint32
	Blksize   uint32
	Padding   uint32
}

type Kstatfs struct {
	Blocks  uint64
	Bfree   uint64
	Bavail  uint64
	Files   uint64
	Ffree   uint64
	Bsize   uint32
	Namelen uint32
	Frsize  uint32
	Padding uint32
	Spare   [6]uint32
}

type FileLock struct {
	Start uint64
	End   uint64
	Typ   uint32
	Pid   uint32 /* tgid */
}

type EntryOut struct {
	NodeId     uint64 /* Inode ID */
	Generation uint64 /* Inode generation: nodeid:gen must
	   be unique for the fs's lifetime */
	EntryValid     uint64 /* Cache timeout for the name */
	AttrValid      uint64 /* Cache timeout for the attributes */
	EntryValidNsec uint32
	AttrValidNsec  uint32
	Attr           Attr
}

type ForgetIn struct {
	Nlookup uint64
}

type GetAttrIn struct {
	GetAttrFlags uint32
	Dummy        uint32
	Fh           uint64
}

type AttrOut struct {
	AttrValid     uint64 /* Cache timeout for the attributes */
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

type SetattrIn struct {
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
	Uid       uint32
	Gid       uint32
	Unused5   uint32
}

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
	Fh         uint64
	Open_flags uint32
	Padding    uint32
}

type ReleaseIn struct {
	Fh            uint64
	Flags         uint32
	Release_flags uint32
	LockOwner     uint64
}

type FlushIn struct {
	Fh        uint64
	Unused    uint32
	Padding   uint32
	LockOwner uint64
}

type ReadIn struct {
	Fh         uint64
	Offset     uint64
	Size       uint32
	Read_flags uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}


type WriteIn struct {
	Fh          uint64
	Offset      uint64
	Size        uint32
	Write_flags uint32
	LockOwner   uint64
	Flags       uint32
	Padding     uint32
}

type WriteOut struct {
	Size    uint32
	Padding uint32
}


type StatfsOut struct {
	St Kstatfs
}

type FsyncIn struct {
	Fh          uint64
	Fsync_flags uint32
	Padding     uint32
}

type SetXattrIn struct {
	Size  uint32
	Flags uint32
}

type GetXattrIn struct {
	Size    uint32
	Padding uint32
}

type GetXattrOut struct {
	Size    uint32
	Padding uint32
}

type LkIn struct {
	Fh       uint64
	Owner    uint64
	Lk       FileLock
	Lk_flags uint32
	Padding  uint32
}

type LkOut struct {
	Lk FileLock
}

type AccessIn struct {
	Mask    uint32
	Padding uint32
}

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
	Max_read uint32
	MaxWrite uint32
	DevMajor uint32 /* chardev major */
	DevMinor uint32 /* chardev minor */
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
	Length  uint32
	Opcode  uint32
	Unique  uint64
	NodeId  uint64
	Uid     uint32
	Gid     uint32
	Pid     uint32
	Padding uint32
}

const SizeOfOutHeader = 16

type OutHeader struct {
	Length uint32
	Error  int32
	Unique uint64
}

type Dirent struct {
	Ino     uint64
	Off     uint64
	Namelen uint32
	Typ     uint32
	//	name []byte // char name[0] -- looks like the name is right after this struct.
}

type NotifyInvalInodeOut struct {
	Ino    uint64
	Off    int64
	Length int64
}

type NotifyInvalEntryOut struct {
	Parent  uint64
	Namelen uint32
	Padding uint32
}
