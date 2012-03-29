package raw


type ForgetIn struct {
	Nlookup uint64
}

type ForgetOne struct {
	NodeId  uint64
	Nlookup uint64
}

type BatchForgetIn struct {
	Count uint32
	Dummy uint32
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
	
type MknodIn struct {
	Mode    uint32
	Rdev    uint32
	Umask   uint32
	Padding uint32
}

type Owner struct {
	Uid uint32
	Gid uint32
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
	// Mask for GetAttrIn.Flags. If set, GetAttrIn has a file handle set.
	FUSE_GETATTR_FH = (1 << 0)
)

type GetAttrIn struct {
	Flags uint32
	Dummy uint32
	Fh    uint64
}


const RELEASE_FLUSH = (1 << 0)

type ReleaseIn struct {
	Fh           uint64
	Flags        uint32
	ReleaseFlags uint32
	LockOwner    uint64
}

type OpenIn struct {
	Flags  uint32
	Unused uint32
}

const (
	// OpenOut.Flags
	FOPEN_DIRECT_IO   = (1 << 0)
	FOPEN_KEEP_CACHE  = (1 << 1)
	FOPEN_NONSEEKABLE = (1 << 2)
)

type OpenOut struct {
	Fh        uint64
	OpenFlags uint32
	Padding   uint32
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

type WriteOut struct {
	Size    uint32
	Padding uint32
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

type FileLock struct {
	Start uint64
	End   uint64
	Typ   uint32
	Pid   uint32
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

// For AccessIn.Mask.
const (
	X_OK = 1
	W_OK = 2
	R_OK = 4
	F_OK = 0
)

type AccessIn struct {
	Mask    uint32
	Padding uint32
}
