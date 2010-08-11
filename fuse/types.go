package fuse

const (

	/** Version number of this interface */
	FUSE_KERNEL_VERSION = 7

	/** Minor version number of this interface */
	FUSE_KERNEL_MINOR_VERSION = 13

	/** The node ID of the root inode */
	FUSE_ROOT_ID = 1

	/**
	 * Bitmasks for Setattr_in.valid
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
	 * Getattr flags
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
	 * FUSE_WRITE_LOCKOWNER: lock_owner field is valid
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

type Notyfy_code int

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

type File_lock struct {
	Start uint64
	End   uint64
	Typ   uint32
	Pid   uint32 /* tgid */
}


type Entry_out struct {
	Nodeid     uint64 /* Inode ID */
	Generation uint64 /* Inode generation: nodeid:gen must
	   be unique for the fs's lifetime */
	Entry_valid      uint64 /* Cache timeout for the name */
	Attr_valid       uint64 /* Cache timeout for the attributes */
	Entry_valid_nsec uint32
	Attr_valid_nsec  uint32
	Attr             Attr
}

type Forget_in struct {
	Nlookup uint64
}

type Getattr_in struct {
	Getattr_flags uint32
	Dummy         uint32
	Fh            uint64
}


type Attr_out struct {
	Attr_valid      uint64 /* Cache timeout for the attributes */
	Attr_valid_nsec uint32
	Dummy           uint32
	Attr            Attr
}


type Mknod_in struct {
	Mode    uint32
	Rdev    uint32
	Umask   uint32
	Padding uint32
}

type Mkdir_in struct {
	Mode  uint32
	Umask uint32
}

type Rename_in struct {
	Newdir uint64
}

type Link_in struct {
	Oldnodeid uint64
}

type Setattr_in struct {
	Valid      uint32
	Padding    uint32
	Fh         uint64
	Size       uint64
	Lock_owner uint64
	Atime      uint64
	Mtime      uint64
	Unused2    uint64
	Atimensec  uint32
	Mtimensec  uint32
	Unused3    uint32
	Mode       uint32
	Unused4    uint32
	Uid        uint32
	Gid        uint32
	Unused5    uint32
}

type Open_in struct {
	Flags  uint32
	Unused uint32
}

type Create_in struct {
	Flags   uint32
	Mode    uint32
	Umask   uint32
	Padding uint32
}

type Open_out struct {
	Fh         uint64
	Open_flags uint32
	Padding    uint32
}

type Release_in struct {
	Fh            uint64
	Flags         uint32
	Release_flags uint32
	Lock_owner    uint64
}

type Flush_in struct {
	Fh         uint64
	Unused     uint32
	Padding    uint32
	Lock_owner uint64
}

type Read_in struct {
	Fh         uint64
	Offset     uint64
	Size       uint32
	Read_flags uint32
	Lock_owner uint64
	Flags      uint32
	Padding    uint32
}


type Write_in struct {
	Fh          uint64
	Offset      uint64
	Size        uint32
	Write_flags uint32
	Lock_owner  uint64
	Flags       uint32
	Padding     uint32
}

type Write_out struct {
	Size    uint32
	Padding uint32
}


type Statfs_out struct {
	St Kstatfs
}

type Fsync_in struct {
	Fh          uint64
	Fsync_flags uint32
	Padding     uint32
}

type Setxattr_in struct {
	Size  uint32
	Flags uint32
}

type Getxattr_in struct {
	Size    uint32
	Padding uint32
}

type Getxattr_out struct {
	Size    uint32
	Padding uint32
}

type Lk_in struct {
	Fh       uint64
	Owner    uint64
	Lk       File_lock
	Lk_flags uint32
	Padding  uint32
}

type Lk_out struct {
	Lk File_lock
}

type Access_in struct {
	Mask    uint32
	Padding uint32
}

type Init_in struct {
	Major         uint32
	Minor         uint32
	Max_readahead uint32
	Flags         uint32
}

type Init_out struct {
	Major                uint32
	Minor                uint32
	Max_readahead        uint32
	Flags                uint32
	Max_background       uint16
	Congestion_threshold uint16
	Max_write            uint32
}

type Cuse_init_in struct {
	Major  uint32
	Minor  uint32
	Unused uint32
	Flags  uint32
}

type Cuse_init_out struct {
	Major     uint32
	Minor     uint32
	Unused    uint32
	Flags     uint32
	Max_read  uint32
	Max_write uint32
	Dev_major uint32 /* chardev major */
	Dev_minor uint32 /* chardev minor */
	Spare     [10]uint32
}

type Interrupt_in struct {
	Unique uint64
}

type Bmap_in struct {
	Block     uint64
	Blocksize uint32
	Padding   uint32
}

type Bmap_out struct {
	Block uint64
}

type Ioctl_in struct {
	Fh       uint64
	Flags    uint32
	Cmd      uint32
	Arg      uint64
	In_size  uint32
	Out_size uint32
}

type Ioctl_out struct {
	Result   int32
	Flags    uint32
	In_iovs  uint32
	Out_iovs uint32
}

type Poll_in struct {
	Fh      uint64
	Kh      uint64
	Flags   uint32
	Padding uint32
}

type Poll_out struct {
	Revents uint32
	Padding uint32
}

type Notify_poll_wakeup_out struct {
	Kh uint64
}

type In_header struct {
	Length  uint32
	Opcode  uint32
	Unique  uint64
	Nodeid  uint64
	Uid     uint32
	Gid     uint32
	Pid     uint32
	Padding uint32
}

type Out_header struct {
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

type Notify_inval_inode_out struct {
	Ino    uint64
	Off    int64
	Length int64
}

type Notify_inval_entry_out struct {
	Parent  uint64
	Namelen uint32
	Padding uint32
}
