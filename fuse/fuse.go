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

)

/* Make sure all structures are padded to 64bit boundary, so 32bit
   userspace works under 64bit kernels */

type fuse_attr struct {
	__u64	ino;
	__u64	size;
	__u64	blocks;
	__u64	atime;
	__u64	mtime;
	__u64	ctime;
	__u32	atimensec;
	__u32	mtimensec;
	__u32	ctimensec;
	__u32	mode;
	__u32	nlink;
	__u32	uid;
	__u32	gid;
	__u32	rdev;
	__u32	blksize;
	__u32	padding;
};

type fuse_kstatfs struct {
	__u64	blocks;
	__u64	bfree;
	__u64	bavail;
	__u64	files;
	__u64	ffree;
	__u32	bsize;
	__u32	namelen;
	__u32	frsize;
	__u32	padding;
	__u32	spare[6];
};

type fuse_file_lock struct {
	__u64	start;
	__u64	end;
	__u32	type;
	__u32	pid; /* tgid */
};

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
};

enum fuse_notify_code {
	FUSE_NOTIFY_POLL   = 1,
	FUSE_NOTIFY_INVAL_INODE = 2,
	FUSE_NOTIFY_INVAL_ENTRY = 3,
	FUSE_NOTIFY_CODE_MAX,
};

type fuse_entry_out struct {
	__u64	nodeid;		/* Inode ID */
	__u64	generation;	/* Inode generation: nodeid:gen must
				   be unique for the fs's lifetime */
	__u64	entry_valid;	/* Cache timeout for the name */
	__u64	attr_valid;	/* Cache timeout for the attributes */
	__u32	entry_valid_nsec;
	__u32	attr_valid_nsec;
	struct fuse_attr attr;
};

type fuse_forget_in struct {
	__u64	nlookup;
};

type fuse_getattr_in struct {
	__u32	getattr_flags;
	__u32	dummy;
	__u64	fh;
};


type fuse_attr_out struct {
	__u64	attr_valid;	/* Cache timeout for the attributes */
	__u32	attr_valid_nsec;
	__u32	dummy;
	struct fuse_attr attr;
};


type fuse_mknod_in struct {
	__u32	mode;
	__u32	rdev;
	__u32	umask;
	__u32	padding;
};

type fuse_mkdir_in struct {
	__u32	mode;
	__u32	umask;
};

type fuse_rename_in struct {
	__u64	newdir;
};

type fuse_link_in struct {
	__u64	oldnodeid;
};

type fuse_setattr_in struct {
	__u32	valid;
	__u32	padding;
	__u64	fh;
	__u64	size;
	__u64	lock_owner;
	__u64	atime;
	__u64	mtime;
	__u64	unused2;
	__u32	atimensec;
	__u32	mtimensec;
	__u32	unused3;
	__u32	mode;
	__u32	unused4;
	__u32	uid;
	__u32	gid;
	__u32	unused5;
};

type fuse_open_in struct {
	__u32	flags;
	__u32	unused;
};

type fuse_create_in struct {
	__u32	flags;
	__u32	mode;
	__u32	umask;
	__u32	padding;
};

type fuse_open_out struct {
	__u64	fh;
	__u32	open_flags;
	__u32	padding;
};

type fuse_release_in struct {
	__u64	fh;
	__u32	flags;
	__u32	release_flags;
	__u64	lock_owner;
};

type fuse_flush_in struct {
	__u64	fh;
	__u32	unused;
	__u32	padding;
	__u64	lock_owner;
};

type fuse_read_in struct {
	__u64	fh;
	__u64	offset;
	__u32	size;
	__u32	read_flags;
	__u64	lock_owner;
	__u32	flags;
	__u32	padding;
};


type fuse_write_in struct {
	__u64	fh;
	__u64	offset;
	__u32	size;
	__u32	write_flags;
	__u64	lock_owner;
	__u32	flags;
	__u32	padding;
};

type fuse_write_out struct {
	__u32	size;
	__u32	padding;
};


type fuse_statfs_out struct {
	struct fuse_kstatfs st;
};

type fuse_fsync_in struct {
	__u64	fh;
	__u32	fsync_flags;
	__u32	padding;
};

type fuse_setxattr_in struct {
	__u32	size;
	__u32	flags;
};

type fuse_getxattr_in struct {
	__u32	size;
	__u32	padding;
};

type fuse_getxattr_out struct {
	__u32	size;
	__u32	padding;
};

type fuse_lk_in struct {
	__u64	fh;
	__u64	owner;
	struct fuse_file_lock lk;
	__u32	lk_flags;
	__u32	padding;
};

type fuse_lk_out struct {
	struct fuse_file_lock lk;
};

type fuse_access_in struct {
	__u32	mask;
	__u32	padding;
};

type fuse_init_in struct {
	__u32	major;
	__u32	minor;
	__u32	max_readahead;
	__u32	flags;
};

type fuse_init_out struct {
	__u32	major;
	__u32	minor;
	__u32	max_readahead;
	__u32	flags;
	__u16   max_background;
	__u16   congestion_threshold;
	__u32	max_write;
};

type cuse_init_in struct {
	__u32	major;
	__u32	minor;
	__u32	unused;
	__u32	flags;
};

type cuse_init_out struct {
	__u32	major;
	__u32	minor;
	__u32	unused;
	__u32	flags;
	__u32	max_read;
	__u32	max_write;
	__u32	dev_major;		/* chardev major */
	__u32	dev_minor;		/* chardev minor */
	__u32	spare[10];
};

type fuse_interrupt_in struct {
	__u64	unique;
};

type fuse_bmap_in struct {
	__u64	block;
	__u32	blocksize;
	__u32	padding;
};

type fuse_bmap_out struct {
	__u64	block;
};

type fuse_ioctl_in struct {
	__u64	fh;
	__u32	flags;
	__u32	cmd;
	__u64	arg;
	__u32	in_size;
	__u32	out_size;
};

type fuse_ioctl_out struct {
	__s32	result;
	__u32	flags;
	__u32	in_iovs;
	__u32	out_iovs;
};

type fuse_poll_in struct {
	__u64	fh;
	__u64	kh;
	__u32	flags;
	__u32   padding;
};

type fuse_poll_out struct {
	__u32	revents;
	__u32	padding;
};

type fuse_notify_poll_wakeup_out struct {
	__u64	kh;
};

type fuse_in_header struct {
	__u32	len;
	__u32	opcode;
	__u64	unique;
	__u64	nodeid;
	__u32	uid;
	__u32	gid;
	__u32	pid;
	__u32	padding;
};

type fuse_out_header struct {
	__u32	len;
	__s32	error;
	__u64	unique;
};

type fuse_dirent struct {
	__u64	ino;
	__u64	off;
	__u32	namelen;
	__u32	type;
	char name[0];
};

type fuse_notify_inval_inode_out struct {
	__u64	ino;
	__s64	off;
	__s64	len;
};

type fuse_notify_inval_entry_out struct {
	__u64	parent;
	__u32	namelen;
	__u32	padding;
};

