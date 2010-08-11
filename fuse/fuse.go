package fuse

const (

/** Version number of this interface */
FUSE_KERNEL_VERSION = 7

/** Minor version number of this interface */
FUSE_KERNEL_MINOR_VERSION = 13

/** The node ID of the root inode */
FUSE_ROOT_ID = 1

/**
 * Bitmasks for fuse_setattr_in.valid
 */
FATTR_MODE = (1 << 0)
FATTR_UID = (1 << 1)
FATTR_GID = (1 << 2)
FATTR_SIZE = (1 << 3)
FATTR_ATIME = (1 << 4)
FATTR_MTIME = (1 << 5)
FATTR_FH = (1 << 6)
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
FOPEN_DIRECT_IO = 	(1 << 0)
FOPEN_KEEP_CACHE = (1 << 1)
FOPEN_NONSEEKABLE = (1 << 2)

/**
 * INIT request/reply flags
 *
 * FUSE_EXPORT_SUPPORT: filesystem handles lookups of "." and ".."
 * FUSE_DONT_MASK: don't apply umask to file mode on create operations
 */
FUSE_ASYNC_READ = 	(1 << 0)
FUSE_POSIX_LOCKS = (1 << 1)
FUSE_FILE_OPS = 	(1 << 2)
FUSE_ATOMIC_O_TRUNC = (1 << 3)
FUSE_EXPORT_SUPPORT = (1 << 4)
FUSE_BIG_WRITES = 	(1 << 5)
FUSE_DONT_MASK = 	(1 << 6)

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
FUSE_GETATTR_FH = 	(1 << 0)

/**
 * Lock flags
 */
FUSE_LK_FLOCK = 	(1 << 0)

/**
 * WRITE flags
 *
 * FUSE_WRITE_CACHE: delayed write from page cache, file handle is guessed
 * FUSE_WRITE_LOCKOWNER: lock_owner field is valid
 */
FUSE_WRITE_CACHE = (1 << 0)
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
FUSE_IOCTL_COMPAT = (1 << 0)
FUSE_IOCTL_UNRESTRICTED = (1 << 1)
FUSE_IOCTL_RETRY = (1 << 2)

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

enum fuse_opcode {
	FUSE_LOOKUP	   = 1,
	FUSE_FORGET	   = 2,  /* no reply */
	FUSE_GETATTR	   = 3,
	FUSE_SETATTR	   = 4,
	FUSE_READLINK	   = 5,
	FUSE_SYMLINK	   = 6,
	FUSE_MKNOD	   = 8,
	FUSE_MKDIR	   = 9,
	FUSE_UNLINK	   = 10,
	FUSE_RMDIR	   = 11,
	FUSE_RENAME	   = 12,
	FUSE_LINK	   = 13,
	FUSE_OPEN	   = 14,
	FUSE_READ	   = 15,
	FUSE_WRITE	   = 16,
	FUSE_STATFS	   = 17,
	FUSE_RELEASE       = 18,
	FUSE_FSYNC         = 20,
	FUSE_SETXATTR      = 21,
	FUSE_GETXATTR      = 22,
	FUSE_LISTXATTR     = 23,
	FUSE_REMOVEXATTR   = 24,
	FUSE_FLUSH         = 25,
	FUSE_INIT          = 26,
	FUSE_OPENDIR       = 27,
	FUSE_READDIR       = 28,
	FUSE_RELEASEDIR    = 29,
	FUSE_FSYNCDIR      = 30,
	FUSE_GETLK         = 31,
	FUSE_SETLK         = 32,
	FUSE_SETLKW        = 33,
	FUSE_ACCESS        = 34,
	FUSE_CREATE        = 35,
	FUSE_INTERRUPT     = 36,
	FUSE_BMAP          = 37,
	FUSE_DESTROY       = 38,
	FUSE_IOCTL         = 39,
	FUSE_POLL          = 40,

	/* CUSE specific operations */
	CUSE_INIT          = 4096,
}

enum fuse_notify_code {
	FUSE_NOTIFY_POLL   = 1,
	FUSE_NOTIFY_INVAL_INODE = 2,
	FUSE_NOTIFY_INVAL_ENTRY = 3,
	FUSE_NOTIFY_CODE_MAX,
}

)

/* Make sure all structures are padded to 64bit boundary, so 32bit
   userspace works under 64bit kernels */

type fuse_attr struct {
	ino uint64
	size uint64
	blocks uint64
	atime uint64
	mtime uint64
	ctime uint64
	atimensec uint32
	mtimensec uint32
	ctimensec uint32
	mode uint32
	nlink uint32
	uid uint32
	gid uint32
	rdev uint32
	blksize uint32
	padding uint32
}

type fuse_kstatfs struct {
	blocks uint64
	bfree uint64
	bavail uint64
	files uint64
	ffree uint64
	bsize uint32
	namelen uint32
	frsize uint32
	padding uint32
	spare [6]uint32
}

type fuse_file_lock struct {
	start uint64
	end uint64
	typ uint32
	pid uint32 /* tgid */
}



type fuse_entry_out struct {
	nodeid uint64		/* Inode ID */
	generation uint64	/* Inode generation: nodeid:gen must
				   be unique for the fs's lifetime */
	entry_valid uint64	/* Cache timeout for the name */
	attr_valid uint64	/* Cache timeout for the attributes */
	entry_valid_nsec uint32
	attr_valid_nsec uint32
	attr fuse_attr
}

type fuse_forget_in struct {
	nlookup uint64
}

type fuse_getattr_in struct {
	getattr_flags uint32
	dummy uint32
	fh uint64
}


type fuse_attr_out struct {
	attr_valid uint64	/* Cache timeout for the attributes */
	attr_valid_nsec uint32
	dummy uint32
	attr fuse_attr
}


type fuse_mknod_in struct {
	mode uint32
	rdev uint32
	umask uint32
	padding uint32
}

type fuse_mkdir_in struct {
	mode uint32
	umask uint32
}

type fuse_rename_in struct {
	newdir uint64
}

type fuse_link_in struct {
	oldnodeid uint64
}

type fuse_setattr_in struct {
	valid uint32
	padding uint32
	fh uint64
	size uint64
	lock_owner uint64
	atime uint64
	mtime uint64
	unused2 uint64
	atimensec uint32
	mtimensec uint32
	unused3 uint32
	mode uint32
	unused4 uint32
	uid uint32
	gid uint32
	unused5 uint32
}

type fuse_open_in struct {
	flags uint32
	unused uint32
}

type fuse_create_in struct {
	flags uint32
	mode uint32
	umask uint32
	padding uint32
}

type fuse_open_out struct {
	fh uint64
	open_flags uint32
	padding uint32
}

type fuse_release_in struct {
	fh uint64
	flags uint32
	release_flags uint32
	lock_owner uint64
}

type fuse_flush_in struct {
	fh uint64
	unused uint32
	padding uint32
	lock_owner uint64
}

type fuse_read_in struct {
	fh uint64
	offset uint64
	size uint32
	read_flags uint32
	lock_owner uint64
	flags uint32
	padding uint32
}


type fuse_write_in struct {
	fh uint64
	offset uint64
	size uint32
	write_flags uint32
	lock_owner uint64
	flags uint32
	padding uint32
}

type fuse_write_out struct {
	size uint32
	padding uint32
}


type fuse_statfs_out struct {
	st fuse_kstatfs
}

type fuse_fsync_in struct {
	fh uint64
	fsync_flags uint32
	padding uint32
}

type fuse_setxattr_in struct {
	size uint32
	flags uint32
}

type fuse_getxattr_in struct {
	size uint32
	padding uint32
}

type fuse_getxattr_out struct {
	size uint32
	padding uint32
}

type fuse_lk_in struct {
	fh uint64
	owner uint64
	lk fuse_file_lock
	lk_flags uint32
	padding uint32
}

type fuse_lk_out struct {
	lk fuse_file_lock
}

type fuse_access_in struct {
	mask uint32
	padding uint32
}

type fuse_init_in struct {
	major uint32
	minor uint32
	max_readahead uint32
	flags uint32
}

type fuse_init_out struct {
	major uint32
	minor uint32
	max_readahead uint32
	flags uint32
	max_background uint16
	congestion_threshold uint16
	max_write uint32
}

type cuse_init_in struct {
	major uint32
	minor uint32
	unused uint32
	flags uint32
}

type cuse_init_out struct {
	major uint32
	minor uint32
	unused uint32
	flags uint32
	max_read uint32
	max_write uint32
	dev_major uint32		/* chardev major */
	dev_minor uint32		/* chardev minor */
	spare [10]uint32
}

type fuse_interrupt_in struct {
	unique uint64
}

type fuse_bmap_in struct {
	block uint64
	blocksize uint32
	padding uint32
}

type fuse_bmap_out struct {
	block uint64
}

type fuse_ioctl_in struct {
	fh uint64
	flags uint32
	cmd uint32
	arg uint64
	in_size uint32
	out_size uint32
}

type fuse_ioctl_out struct {
	result __s32
	flags uint32
	in_iovs uint32
	out_iovs uint32
}

type fuse_poll_in struct {
	fh uint64
	kh uint64
	flags uint32
	padding uint32
}

type fuse_poll_out struct {
	revents uint32
	padding uint32
}

type fuse_notify_poll_wakeup_out struct {
	kh uint64
}

type fuse_in_header struct {
	len uint32
	opcode uint32
	unique uint64
	nodeid uint64
	uid uint32
	gid uint32
	pid uint32
	padding uint32
}

type fuse_out_header struct {
	len uint32
	error __s32
	unique uint64
}

type fuse_dirent struct {
	ino uint64
	off uint64
	namelen uint32
	typ uint32
//	name []byte // char name[0] -- looks like the name is right after this struct.
}

type fuse_notify_inval_inode_out struct {
	ino uint64
	off __s64
	len __s64
}

type fuse_notify_inval_entry_out struct {
	parent uint64
	namelen uint32
	padding uint32
}

