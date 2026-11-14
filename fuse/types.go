// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	_DEFAULT_BACKGROUND_TASKS = 12
)

// Errno values for convenience. Any syscall.Errno can be returned.
const (
	// OK indicates success.
	OK = syscall.Errno(0)

	// EACCESS Permission denied
	EACCES = syscall.EACCES

	// EBUSY Device or resource busy
	EBUSY = syscall.EBUSY

	// EAGAIN Resource temporarily unavailable
	EAGAIN = syscall.EAGAIN

	// EINTR Call was interrupted
	EINTR = syscall.EINTR

	// EINVAL Invalid argument
	EINVAL = syscall.EINVAL

	// EIO I/O error
	EIO = syscall.EIO

	// ENOENT No such file or directory
	ENOENT = syscall.ENOENT

	// ENOSYS Function not implemented
	ENOSYS = syscall.ENOSYS

	// ENOTDIR Not a directory
	ENOTDIR = syscall.ENOTDIR

	// ENOTSUP Not supported
	ENOTSUP = syscall.ENOTSUP

	// EISDIR Is a directory
	EISDIR = syscall.EISDIR

	// EPERM Operation not permitted
	EPERM = syscall.EPERM

	// ERANGE Math result not representable
	ERANGE = syscall.ERANGE

	// EXDEV Cross-device link
	EXDEV = syscall.EXDEV

	// EBADF Bad file number
	EBADF = syscall.EBADF

	// ENODEV No such device
	ENODEV = syscall.ENODEV

	// EROFS Read-only file system
	EROFS = syscall.EROFS
)

type ForgetIn struct {
	InHeader

	Nlookup uint64
}

// batch forget is handled internally.
type _ForgetOne struct {
	NodeId  uint64
	Nlookup uint64
}

// batch forget is handled internally.
type _BatchForgetIn struct {
	InHeader
	Count uint32
	Dummy uint32
}

type MkdirIn struct {
	InHeader

	// The mode for the new directory. The calling process' umask
	// is already factored into the mode.
	Mode  uint32
	Umask uint32
}

type RenameIn struct {
	InHeader
	Newdir  uint64
	Flags   uint32
	Padding uint32
}

const compatRenameInSize = int(unsafe.Sizeof(InHeader{})) + 8

func renameInSize(negotiated uint64) int {
	if negotiated&CAP_RENAME_SWAP == 0 {
		return compatRenameInSize
	}
	return int(unsafe.Sizeof(RenameIn{}))
}

type LinkIn struct {
	InHeader
	Oldnodeid uint64
}

type Owner struct {
	Uid uint32
	Gid uint32
}

const ( // SetAttrIn.Valid
	FATTR_MODE         = (1 << 0)
	FATTR_UID          = (1 << 1)
	FATTR_GID          = (1 << 2)
	FATTR_SIZE         = (1 << 3)
	FATTR_ATIME        = (1 << 4)
	FATTR_MTIME        = (1 << 5)
	FATTR_FH           = (1 << 6)
	FATTR_ATIME_NOW    = (1 << 7)
	FATTR_MTIME_NOW    = (1 << 8)
	FATTR_LOCKOWNER    = (1 << 9)
	FATTR_CTIME        = (1 << 10)
	FATTR_KILL_SUIDGID = (1 << 11)
)

type SetAttrInCommon struct {
	InHeader

	Valid     uint32
	Padding   uint32
	Fh        uint64
	Size      uint64
	LockOwner uint64
	Atime     uint64
	Mtime     uint64
	Ctime     uint64
	Atimensec uint32
	Mtimensec uint32
	Ctimensec uint32
	Mode      uint32
	Unused4   uint32
	Owner
	Unused5 uint32
}

// GetFh returns the file handle if available, or 0 if undefined.
func (in *SetAttrInCommon) GetFh() (uint64, bool) {
	if in.Valid&FATTR_FH != 0 {
		return in.Fh, true
	}
	return 0, false
}

func (in *SetAttrInCommon) GetMode() (uint32, bool) {
	if in.Valid&FATTR_MODE != 0 {
		return in.Mode & 07777, true
	}
	return 0, false
}

func (in *SetAttrInCommon) GetUID() (uint32, bool) {
	if in.Valid&FATTR_UID != 0 {
		return in.Uid, true
	}
	return ^uint32(0), false
}

func (in *SetAttrInCommon) GetGID() (uint32, bool) {
	if in.Valid&FATTR_GID != 0 {
		return in.Gid, true
	}
	return ^uint32(0), false
}

func (in *SetAttrInCommon) GetSize() (uint64, bool) {
	if in.Valid&FATTR_SIZE != 0 {
		return in.Size, true
	}
	return 0, false
}

func (in *SetAttrInCommon) GetMTime() (time.Time, bool) {
	var t time.Time
	if in.Valid&FATTR_MTIME != 0 {
		if in.Valid&FATTR_MTIME_NOW != 0 {
			t = time.Now()
		} else {
			t = time.Unix(int64(in.Mtime), int64(in.Mtimensec))
		}
		return t, true
	}

	return t, false
}

func (in *SetAttrInCommon) GetATime() (time.Time, bool) {
	var t time.Time
	if in.Valid&FATTR_ATIME != 0 {
		if in.Valid&FATTR_ATIME_NOW != 0 {
			t = time.Now()
		} else {
			t = time.Unix(int64(in.Atime), int64(in.Atimensec))
		}
		return t, true
	}

	return t, false
}

func (in *SetAttrInCommon) GetCTime() (time.Time, bool) {
	var t time.Time
	if in.Valid&FATTR_CTIME != 0 {
		t = time.Unix(int64(in.Ctime), int64(in.Ctimensec))
		return t, true
	}

	return t, false
}

const (
	RELEASE_FLUSH        = (1 << 0)
	RELEASE_FLOCK_UNLOCK = (1 << 1)
)

type ReleaseIn struct {
	InHeader
	Fh           uint64
	Flags        uint32
	ReleaseFlags uint32
	LockOwner    uint64
}

type OpenIn struct {
	InHeader
	Flags uint32

	// Mode holds open_flags (OPEN_*), not a file mode.
	Mode uint32
}

const (
	// OpenIn.Mode, CreateIn.OpenFlags: O_TRUNC by an unprivileged caller; clear suid/sgid (CAP_HANDLE_KILLPRIV_V2).
	OPEN_KILL_SUIDGID = (1 << 0)
)

const (
	// OpenOut.Flags
	FOPEN_DIRECT_IO              = (1 << 0)
	FOPEN_KEEP_CACHE             = (1 << 1)
	FOPEN_NONSEEKABLE            = (1 << 2)
	FOPEN_CACHE_DIR              = (1 << 3)
	FOPEN_STREAM                 = (1 << 4)
	FOPEN_NOFLUSH                = (1 << 5)
	FOPEN_PARALLEL_DIRECT_WRITES = (1 << 6)
	FOPEN_PASSTHROUGH            = (1 << 7)
	FOPEN_IO_URING_ZERO_COPY     = (1 << 8)
)

type OpenOut struct {
	Fh        uint64
	OpenFlags uint32
	BackingID int32
}

// To be set in InitIn/InitOut.Flags.
//
// Keep in sync with either of
// * https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/uapi/linux/fuse.h
// * https://github.com/libfuse/libfuse/blob/master/include/fuse_kernel.h
// but NOT with
// * https://github.com/libfuse/libfuse/blob/master/include/fuse_common.h
// This file has CAP_HANDLE_KILLPRIV and CAP_POSIX_ACL reversed!
const (
	CAP_ASYNC_READ       = (1 << 0)
	CAP_POSIX_LOCKS      = (1 << 1)
	CAP_FILE_OPS         = (1 << 2)
	CAP_ATOMIC_O_TRUNC   = (1 << 3)
	CAP_EXPORT_SUPPORT   = (1 << 4)
	CAP_BIG_WRITES       = (1 << 5)
	CAP_DONT_MASK        = (1 << 6)
	CAP_SPLICE_WRITE     = (1 << 7)
	CAP_SPLICE_MOVE      = (1 << 8)
	CAP_SPLICE_READ      = (1 << 9)
	CAP_FLOCK_LOCKS      = (1 << 10)
	CAP_IOCTL_DIR        = (1 << 11)
	CAP_AUTO_INVAL_DATA  = (1 << 12) // mtime changes invalidate page cache.
	CAP_READDIRPLUS      = (1 << 13)
	CAP_READDIRPLUS_AUTO = (1 << 14)
	CAP_ASYNC_DIO        = (1 << 15)
	CAP_WRITEBACK_CACHE  = (1 << 16)
	CAP_NO_OPEN_SUPPORT  = (1 << 17)
	CAP_PARALLEL_DIROPS  = (1 << 18)
	CAP_HANDLE_KILLPRIV  = (1 << 19)
	CAP_POSIX_ACL        = (1 << 20)
	CAP_ABORT_ERROR      = (1 << 21)
	CAP_MAX_PAGES        = (1 << 22)
	CAP_CACHE_SYMLINKS   = (1 << 23)

	/* bits 24..31 differ across linux and mac */
	/* bits 32..63 get shifted down 32 bits into the Flags2 field */
	CAP_SECURITY_CTX         = (1 << 32)
	CAP_HAS_INODE_DAX        = (1 << 33)
	CAP_CREATE_SUPP_GROUP    = (1 << 34)
	CAP_HAS_EXPIRE_ONLY      = (1 << 35)
	CAP_DIRECT_IO_ALLOW_MMAP = (1 << 36)
	CAP_PASSTHROUGH          = (1 << 37)
	CAP_NO_EXPORT_SUPPORT    = (1 << 38)
	CAP_HAS_RESEND           = (1 << 39)
	CAP_ALLOW_IDMAP          = (1 << 40)
	CAP_OVER_IO_URING        = (1 << 41)
	CAP_REQUEST_TIMEOUT      = (1 << 42)
	CAP_HAS_IO_URING_BUFPOOL = (1 << 43)
	CAP_HAS_SYNCFS           = (1 << 44)
	CAP_PASSTHROUGH_V2       = (1 << 45)

	// knownCapabilities has the capability bits that this
	// version has names for.
	knownCapabilities = CAP_PASSTHROUGH_V2<<1 - 1
)

type InitIn struct {
	InHeader

	Major        uint32
	Minor        uint32
	MaxReadAhead uint32
	Flags        uint32
	Flags2       uint32
	Unused       [11]uint32
}

func (in *InitIn) Flags64() uint64 {
	return uint64(in.Flags) | uint64(in.Flags2)<<32
}

type InitOut struct {
	Major               uint32
	Minor               uint32
	MaxReadAhead        uint32
	Flags               uint32
	MaxBackground       uint16
	CongestionThreshold uint16
	MaxWrite            uint32
	TimeGran            uint32
	MaxPages            uint16
	MapAlignment        uint16
	Flags2              uint32
	MaxStackDepth       uint32
	RequestTimeout      uint16

	Unused [11]uint16
}

func (o *InitOut) Flags64() uint64 {
	return uint64(o.Flags) | uint64(o.Flags2)<<32
}

type _CuseInitIn struct {
	InHeader
	Major  uint32
	Minor  uint32
	Unused uint32
	Flags  uint32
}

type _CuseInitOut struct {
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
	InHeader
	Unique uint64
}

type _BmapIn struct {
	InHeader
	Block     uint64
	Blocksize uint32
	Padding   uint32
}

type _BmapOut struct {
	Block uint64
}

const (
	IOCTL_COMPAT       = (1 << 0)
	IOCTL_UNRESTRICTED = (1 << 1)
	IOCTL_RETRY        = (1 << 2)
	IOCTL_32BIT        = (1 << 3)
	IOCTL_DIR          = (1 << 4)
	IOCTL_COMPAT_X32   = (1 << 5)
)

type IoctlIn struct {
	InHeader
	Fh    uint64
	Flags uint32

	// The command, consisting of 16-bits metadata (direction + size) and 16-bits to encode the ioctl number
	Cmd uint32

	// The uint64 argument. If there is a payload, this is the
	// payload address in the caller, and should not be used.
	Arg uint64

	// Size for the payload, non-zero if Cmd is WRITE or READ/WRITE.
	InSize uint32

	// Size for the payload, non-zero if Cmd is READ or READ/WRITE.
	OutSize uint32
}

type IoctlOut struct {
	Result int32

	// The following fields are used for unrestricted ioctls,
	// which are only enabled on CUSE.
	Flags   uint32
	InIovs  uint32
	OutIovs uint32
}

type _PollIn struct {
	InHeader
	Fh      uint64
	Kh      uint64
	Flags   uint32
	Padding uint32
}

type _PollOut struct {
	Revents uint32
	Padding uint32
}

type _NotifyPollWakeupOut struct {
	Kh uint64
}

type WriteOut struct {
	Size    uint32
	Padding uint32
}

type GetXAttrOut struct {
	Size    uint32
	Padding uint32
}

type FileLock struct {
	Start uint64
	End   uint64
	Typ   uint32
	Pid   uint32
}

const LK_FLOCK = (1 << 0)

type LkIn struct {
	InHeader
	Fh      uint64
	Owner   uint64
	Lk      FileLock
	LkFlags uint32
	Padding uint32
}

type LkOut struct {
	Lk FileLock
}

// For AccessIn.Mask.
const (
	X_OK = 1
	W_OK = 2
	R_OK = 4
	F_OK = 0
)

type AccessIn struct {
	InHeader
	Mask    uint32
	Padding uint32
}

// FsyncIn.FsyncFlags
const FSYNC_FDATASYNC = (1 << 0)

type FsyncIn struct {
	InHeader
	Fh         uint64
	FsyncFlags uint32
	Padding    uint32
}

type OutHeader struct {
	Length uint32
	Status int32
	Unique uint64
}

type NotifyInvalInodeOut struct {
	Ino    uint64
	Off    int64
	Length int64
}

type NotifyInvalEntryOut struct {
	Parent  uint64
	NameLen uint32
	Flags   uint32
}

const (
	// NotifyInvalEntryOut.Flags: only expire the entry's timeout,
	// rather than dropping it from the dcache. Needs
	// CAP_HAS_EXPIRE_ONLY.
	EXPIRE_ONLY = (1 << 0)
)

type NotifyInvalDeleteOut struct {
	Parent  uint64
	Child   uint64
	NameLen uint32
	Padding uint32
}

type NotifyStoreOut struct {
	Nodeid  uint64
	Offset  uint64
	Size    uint32
	Padding uint32
}

type NotifyRetrieveOut struct {
	NotifyUnique uint64
	Nodeid       uint64
	Offset       uint64
	Size         uint32
	Padding      uint32
}

type NotifyRetrieveIn struct {
	InHeader
	Dummy1 uint64
	Offset uint64
	Size   uint32
	Dummy2 uint32
	Dummy3 uint64
	Dummy4 uint64
}

type NotifyPruneOut struct {
	Count    uint32
	_padding uint32
	_spare   uint64
}

// notifyCode is the kernel's enum fuse_notify_code. It goes in
// OutHeader.Status of a message with Unique == 0.
type notifyCode int32

const (
	notifyPoll       notifyCode = 1
	notifyInvalInode notifyCode = 2
	notifyInvalEntry notifyCode = 3
	notifyStore      notifyCode = 4
	notifyRetrieve   notifyCode = 5
	notifyDelete     notifyCode = 6
	notifyResend     notifyCode = 7
	notifyIncEpoch   notifyCode = 8
	notifyPrune      notifyCode = 9

	notifyBackingRemove notifyCode = 10
	notifyBackingMap    notifyCode = 11
)

var notifyNames = [...]string{
	notifyPoll:       "NOTIFY_POLL",
	notifyInvalInode: "NOTIFY_INVAL_INODE",
	notifyInvalEntry: "NOTIFY_INVAL_ENTRY",
	notifyStore:      "NOTIFY_STORE_CACHE",
	notifyRetrieve:   "NOTIFY_RETRIEVE_CACHE",
	notifyDelete:     "NOTIFY_DELETE",
	notifyResend:     "NOTIFY_RESEND",
	notifyIncEpoch:   "NOTIFY_INC_EPOCH",
	notifyPrune:      "NOTIFY_PRUNE",

	notifyBackingRemove: "NOTIFY_BACKING_REMOVE",
	notifyBackingMap:    "NOTIFY_BACKING_MAP",
}

func (c notifyCode) String() string {
	if c > 0 && int(c) < len(notifyNames) {
		return notifyNames[c]
	}
	return fmt.Sprintf("notify(%d)", int32(c))
}

type FlushIn struct {
	InHeader
	Fh        uint64
	Unused    uint32
	Padding   uint32
	LockOwner uint64
}

type LseekIn struct {
	InHeader
	Fh      uint64
	Offset  uint64
	Whence  uint32
	Padding uint32
}

type LseekOut struct {
	Offset uint64
}

type CopyFileRangeIn struct {
	InHeader
	FhIn      uint64
	OffIn     uint64
	NodeIdOut uint64
	FhOut     uint64
	OffOut    uint64
	Len       uint64
	Flags     uint64
}

// EntryOut holds the result of a (directory,name) lookup.  It has two
// TTLs, one for the (directory, name) lookup itself, and one for the
// attributes (eg. size, mode). The entry TTL also applies if the
// lookup result is ENOENT ("negative entry lookup")
type EntryOut struct {
	NodeId         uint64
	Generation     uint64
	EntryValid     uint64
	AttrValid      uint64
	EntryValidNsec uint32
	AttrValidNsec  uint32
	Attr
}

// EntryTimeout returns the timeout in nanoseconds for a directory
// entry (existence or non-existence of a file within a directory).
func (o *EntryOut) EntryTimeout() time.Duration {
	return time.Duration(uint64(o.EntryValidNsec) + o.EntryValid*1e9)
}

// AttrTimeout returns the TTL in nanoseconds of the attribute data.
func (o *EntryOut) AttrTimeout() time.Duration {
	return time.Duration(uint64(o.AttrValidNsec) + o.AttrValid*1e9)
}

// SetEntryTimeout sets the entry TTL.
func (o *EntryOut) SetEntryTimeout(dt time.Duration) {
	ns := int64(dt)
	o.EntryValidNsec = uint32(ns % 1e9)
	o.EntryValid = uint64(ns / 1e9)
}

// SetAttrTimeout sets the attribute TTL.
func (o *EntryOut) SetAttrTimeout(dt time.Duration) {
	ns := int64(dt)
	o.AttrValidNsec = uint32(ns % 1e9)
	o.AttrValid = uint64(ns / 1e9)
}

// AttrOut is the type returned by the Getattr call.
type AttrOut struct {
	AttrValid     uint64
	AttrValidNsec uint32
	Dummy         uint32
	Attr
}

func (o *AttrOut) Timeout() time.Duration {
	return time.Duration(uint64(o.AttrValidNsec) + o.AttrValid*1e9)
}

func (o *AttrOut) SetTimeout(dt time.Duration) {
	ns := int64(dt)
	o.AttrValidNsec = uint32(ns % 1e9)
	o.AttrValid = uint64(ns / 1e9)
}

type CreateOut struct {
	EntryOut
	OpenOut
}

// Caller has data on the process making the FS call.
//
// The UID and GID are effective UID/GID, except for the ACCESS
// opcode, where UID and GID are the real UIDs
type Caller struct {
	Owner
	Pid uint32
}

type InHeader struct {
	Length uint32
	Opcode uint32
	Unique uint64
	NodeId uint64
	Caller
	TotalExtlen uint16 // in 8-byte units
	Padding     uint16
}

type StatfsOut struct {
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

// _Dirent is what we send to the kernel, but we offer DirEntry and
// DirEntryList to the user.
type _Dirent struct {
	Ino     uint64
	Off     uint64
	NameLen uint32
	Typ     uint32
}

const (
	READ_LOCKOWNER = (1 << 1)
)

const (
	WRITE_CACHE        = (1 << 0)
	WRITE_LOCKOWNER    = (1 << 1)
	WRITE_KILL_SUIDGID = (1 << 2)
)

type FallocateIn struct {
	InHeader
	Fh      uint64
	Offset  uint64
	Length  uint64
	Mode    uint32
	Padding uint32
}

const (
	// Mask for GetAttrIn.Flags. If set, GetAttrIn has a file handle set.
	FUSE_GETATTR_FH = (1 << 0)
)

type GetAttrIn struct {
	InHeader

	Flags_ uint32
	Dummy  uint32
	Fh_    uint64
}

// Flags accesses the flags. This is a method, because OSXFuse does not
// have GetAttrIn flags.
func (in *GetAttrIn) Flags() uint32 {
	return in.Flags_
}

// Fh accesses the file handle. This is a method, because OSXFuse does not
// have GetAttrIn flags.
func (in *GetAttrIn) Fh() uint64 {
	return in.Fh_
}

type MknodIn struct {
	InHeader

	// Mode to use, including the Umask value
	Mode    uint32
	Rdev    uint32
	Umask   uint32
	Padding uint32
}

type CreateIn struct {
	InHeader
	Flags uint32

	// Mode for the new file; already takes Umask into account.
	Mode uint32

	// Umask used for this create call.
	Umask uint32

	OpenFlags uint32
}

type ReadIn struct {
	InHeader
	Fh        uint64
	Offset    uint64
	Size      uint32
	ReadFlags uint32
	LockOwner uint64
	Flags     uint32
	Padding   uint32
}

type WriteIn struct {
	InHeader
	Fh         uint64
	Offset     uint64
	Size       uint32
	WriteFlags uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}

// Data for registering a file as backing an inode.
const (
	// Flags for FUSE_NOTIFY_BACKING_MAP.
	BACKING_MAP_CREATE = (1 << 0)
	BACKING_MAP_CYCLIC = (1 << 1)

	// MAX_EXTENTS is the maximum number of extents in a
	// FUSE_NOTIFY_BACKING_MAP message.
	MAX_EXTENTS = 1365

	// Flags for FUSE_SETUPMAPPING.
	SETUPMAPPING_FLAG_WRITE = (1 << 0)
	SETUPMAPPING_FLAG_READ  = (1 << 1)

	// INVALID_UIDGID is sent as uid/gid if the caller's ID has no
	// mapping in the mount's idmap (CAP_ALLOW_IDMAP).
	INVALID_UIDGID = ^uint32(0)

	// _UNIQUE_RESEND is set in InHeader.Unique for requests
	// resent after FUSE_NOTIFY_RESEND.
	_UNIQUE_RESEND = (1 << 63)
)

type BackingMap struct {
	Fd      int32
	Flags   uint32
	padding uint64
}

type SxTime struct {
	Sec       uint64
	Nsec      uint32
	_reserved uint32
}

func (t *SxTime) Seconds() float64 {
	return ft(t.Sec, t.Nsec)
}

type Statx struct {
	Mask       uint32
	Blksize    uint32
	Attributes uint64
	Nlink      uint32

	Uid            uint32
	Gid            uint32
	Mode           uint16
	_spare0        uint16
	Ino            uint64
	Size           uint64
	Blocks         uint64
	AttributesMask uint64

	Atime SxTime
	Btime SxTime
	Ctime SxTime
	Mtime SxTime

	RdevMajor uint32
	RdevMinor uint32
	DevMajor  uint32
	DevMinor  uint32
	_spare2   [14]uint64
}

type StatxIn struct {
	InHeader

	GetattrFlags uint32
	_reserved    uint32
	Fh           uint64
	SxFlags      uint32
	SxMask       uint32
}

type StatxOut struct {
	AttrValid     uint64
	AttrValidNsec uint32
	Flags         uint32
	_spare        [2]uint64

	Statx
}

func (o *StatxOut) Timeout() time.Duration {
	return time.Duration(uint64(o.AttrValidNsec) + o.AttrValid*1e9)
}

func (o *StatxOut) SetTimeout(dt time.Duration) {
	ns := int64(dt)
	o.AttrValidNsec = uint32(ns % 1e9)
	o.AttrValid = uint64(ns / 1e9)
}

type CopyFileRangeOut struct {
	BytesCopied uint64
}
