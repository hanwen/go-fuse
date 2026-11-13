// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

package winfsp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// api holds the WinFsp DLL entry points.
type api struct {
	dll *windows.LazyDLL

	fileSystemCreate        *windows.LazyProc
	fileSystemDelete        *windows.LazyProc
	fileSystemSetMountPoint *windows.LazyProc
	fileSystemMountPointF   *windows.LazyProc
	fileSystemStartDisp     *windows.LazyProc
	fileSystemStopDisp      *windows.LazyProc
	fileSystemNotifyBegin   *windows.LazyProc
	fileSystemNotifyEnd     *windows.LazyProc
	fileSystemNotify        *windows.LazyProc
	posixMapPermsToSD       *windows.LazyProc
	posixMapSDToPerms       *windows.LazyProc
	posixMapSidToUid        *windows.LazyProc
	setSecurityDescriptor   *windows.LazyProc
	deleteSecurityDesc      *windows.LazyProc
	findReparsePoint        *windows.LazyProc
	resolveReparsePoints    *windows.LazyProc
}

var (
	apiOnce sync.Once
	theAPI  *api
	apiErr  error
)

// dllPath finds the WinFsp DLL from the registry key that the
// installer writes.
func dllPath() (string, error) {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "a64"
	}
	name := "winfsp-" + arch + ".dll"
	for _, p := range []string{`SOFTWARE\WOW6432Node\WinFsp`, `SOFTWARE\WinFsp`} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, p, registry.QUERY_VALUE|registry.WOW64_32KEY)
		if err != nil {
			continue
		}
		dir, _, err := k.GetStringValue("InstallDir")
		k.Close()
		if err == nil {
			return filepath.Join(dir, "bin", name), nil
		}
	}
	return "", fmt.Errorf("winfsp: WinFsp is not installed (no InstallDir in registry)")
}

func loadAPI() (*api, error) {
	apiOnce.Do(func() {
		path, err := dllPath()
		if err != nil {
			apiErr = err
			return
		}
		dll := windows.NewLazyDLL(path)
		if err := dll.Load(); err != nil {
			apiErr = fmt.Errorf("winfsp: load %s: %w", path, err)
			return
		}
		a := &api{dll: dll}
		for _, p := range []struct {
			dst  **windows.LazyProc
			name string
		}{
			{&a.fileSystemCreate, "FspFileSystemCreate"},
			{&a.fileSystemDelete, "FspFileSystemDelete"},
			{&a.fileSystemSetMountPoint, "FspFileSystemSetMountPoint"},
			{&a.fileSystemMountPointF, "FspFileSystemMountPointF"},
			{&a.fileSystemStartDisp, "FspFileSystemStartDispatcher"},
			{&a.fileSystemStopDisp, "FspFileSystemStopDispatcher"},
			{&a.fileSystemNotifyBegin, "FspFileSystemNotifyBegin"},
			{&a.fileSystemNotifyEnd, "FspFileSystemNotifyEnd"},
			{&a.fileSystemNotify, "FspFileSystemNotify"},
			{&a.posixMapPermsToSD, "FspPosixMapPermissionsToSecurityDescriptor"},
			{&a.posixMapSDToPerms, "FspPosixMapSecurityDescriptorToPermissions"},
			{&a.posixMapSidToUid, "FspPosixMapSidToUid"},
			{&a.setSecurityDescriptor, "FspSetSecurityDescriptor"},
			{&a.deleteSecurityDesc, "FspDeleteSecurityDescriptor"},
			{&a.findReparsePoint, "FspFileSystemFindReparsePoint"},
			{&a.resolveReparsePoints, "FspFileSystemResolveReparsePoints"},
		} {
			*p.dst = dll.NewProc(p.name)
			if err := (*p.dst).Find(); err != nil {
				apiErr = fmt.Errorf("winfsp: %s: %w", path, err)
				return
			}
		}
		theAPI = a
	})
	return theAPI, apiErr
}

// Available reports whether WinFsp is installed.
func Available() bool {
	_, err := loadAPI()
	return err == nil
}

// volumeParams mirrors FSP_FSCTL_VOLUME_PARAMS. Flags and Flags1 hold
// the C bit fields.
type volumeParams struct {
	Version                  uint16 // sizeof(FSP_FSCTL_VOLUME_PARAMS)
	SectorSize               uint16
	SectorsPerAllocationUnit uint16
	MaxComponentLength       uint16
	VolumeCreationTime       uint64
	VolumeSerialNumber       uint32
	TransactTimeout          uint32
	IrpTimeout               uint32
	IrpCapacity              uint32
	FileInfoTimeout          uint32
	Flags                    uint32
	Prefix                   [192]uint16
	FileSystemName           [16]uint16
	Flags1                   uint32
	VolumeInfoTimeout        uint32
	DirInfoTimeout           uint32
	SecurityTimeout          uint32
	StreamInfoTimeout        uint32
	EaTimeout                uint32
	FsextControlCode         uint32
	Reserved32               [1]uint32
	Reserved64               [2]uint64
}

// volumeParams must have the size of FSP_FSCTL_VOLUME_PARAMS.
var _ [504]byte = [unsafe.Sizeof(volumeParams{})]byte{}

// Bits in volumeParams.Flags, and (vf1*) in volumeParams.Flags1.
const (
	vfCaseSensitiveSearch         = 1 << 0
	vfCasePreservedNames          = 1 << 1
	vfUnicodeOnDisk               = 1 << 2
	vfPersistentAcls              = 1 << 3
	vfReparsePoints               = 1 << 4
	vfReparsePointsAccessCheck    = 1 << 5
	vfReadOnlyVolume              = 1 << 9
	vfPostCleanupWhenModifiedOnly = 1 << 10
	vfPassQueryDirectoryFileName  = 1 << 13
	vfUmFileContextIsUserContext2 = 1 << 16
	vfUmNoReparsePointsDirCheck   = 1 << 18

	vf1DirInfoTimeoutValid = 1 << 1
)

// Indices in FSP_FILE_SYSTEM_INTERFACE.
const (
	ifGetVolumeInfo        = 0
	ifSetVolumeLabel       = 1
	ifGetSecurityByName    = 2
	ifCreate               = 3
	ifOpen                 = 4
	ifOverwrite            = 5
	ifCleanup              = 6
	ifClose                = 7
	ifRead                 = 8
	ifWrite                = 9
	ifFlush                = 10
	ifGetFileInfo          = 11
	ifSetBasicInfo         = 12
	ifSetFileSize          = 13
	ifCanDelete            = 14
	ifRename               = 15
	ifGetSecurity          = 16
	ifSetSecurity          = 17
	ifReadDirectory        = 18
	ifResolveReparsePoints = 19
	ifGetReparsePoint      = 20
	ifSetReparsePoint      = 21
	ifGetDirInfoByName     = 24
	ifDispatcherStopped    = 32
	ifCount                = 64
)

// Only amd64 and arm64 are supported (see the build constraint). WinFsp
// also ships winfsp-x86.dll, but supporting 386 needs two changes:
//
//   - windows.NewCallback panics on arguments larger than a uintptr
//     ("compileCallback: argument size is larger than uintptr"). Read,
//     Write, Overwrite, SetFileSize and SetBasicInfo take UINT64
//     arguments, which a 32-bit C caller passes as two stack slots, low
//     half first. Their 386 variants would take each as two uint32
//     halves, and recombine them as hi<<32 | lo.
//   - FSP_FILE_SYSTEM_INTERFACE declares no calling convention, and
//     WinFsp is built with the MSVC default, so the callbacks are
//     __cdecl. NewCallback creates stdcall callbacks, which pop their
//     own arguments; the cdecl caller then pops them again. 386 needs
//     NewCallbackCDecl. On amd64 and arm64 there is only one
//     convention.
//
// Calls into the DLL need no change, because Go's 386 syscall code
// restores the stack pointer after the call. cgofuse does both of the
// above for WinFsp's FUSE API (fuse/host_nocgo_windows.go), with a
// ...32 and a ...64 variant of each callback.

// The callbacks are shared by all mounts, because Windows limits the
// number of callbacks a process can create. The interface table is a
// global so that it never moves or gets collected.
var (
	ifaceOnce sync.Once
	iface     [ifCount]uintptr

	getReparsePointByNameCB uintptr
)

func initInterface() {
	ifaceOnce.Do(func() {
		iface[ifGetVolumeInfo] = windows.NewCallback(cbGetVolumeInfo)
		iface[ifSetVolumeLabel] = windows.NewCallback(cbSetVolumeLabel)
		iface[ifGetSecurityByName] = windows.NewCallback(cbGetSecurityByName)
		iface[ifCreate] = windows.NewCallback(cbCreate)
		iface[ifOpen] = windows.NewCallback(cbOpen)
		iface[ifOverwrite] = windows.NewCallback(cbOverwrite)
		iface[ifCleanup] = windows.NewCallback(cbCleanup)
		iface[ifClose] = windows.NewCallback(cbClose)
		iface[ifRead] = windows.NewCallback(cbRead)
		iface[ifWrite] = windows.NewCallback(cbWrite)
		iface[ifFlush] = windows.NewCallback(cbFlush)
		iface[ifGetFileInfo] = windows.NewCallback(cbGetFileInfo)
		iface[ifSetBasicInfo] = windows.NewCallback(cbSetBasicInfo)
		iface[ifSetFileSize] = windows.NewCallback(cbSetFileSize)
		iface[ifCanDelete] = windows.NewCallback(cbCanDelete)
		iface[ifRename] = windows.NewCallback(cbRename)
		iface[ifGetSecurity] = windows.NewCallback(cbGetSecurity)
		iface[ifSetSecurity] = windows.NewCallback(cbSetSecurity)
		iface[ifReadDirectory] = windows.NewCallback(cbReadDirectory)
		iface[ifResolveReparsePoints] = windows.NewCallback(cbResolveReparsePoints)
		iface[ifGetReparsePoint] = windows.NewCallback(cbGetReparsePoint)
		iface[ifSetReparsePoint] = windows.NewCallback(cbSetReparsePoint)
		iface[ifGetDirInfoByName] = windows.NewCallback(cbGetDirInfoByName)
		iface[ifDispatcherStopped] = windows.NewCallback(cbDispatcherStopped)
		getReparsePointByNameCB = windows.NewCallback(cbGetReparsePointByName)
	})
}

// Server is a mounted WinFsp file system.
type Server struct {
	api   *api
	fs    *fileSystem
	fsp   unsafe.Pointer // FSP_FILE_SYSTEM*
	id    uintptr
	mount string

	// security descriptor mapping
	uid, gid uint32

	done    chan struct{}
	unmount sync.Once

	// fspMu guards s.fsp against FspFileSystemDelete, for
	// notifications that are sent outside callbacks.
	fspMu   sync.RWMutex
	deleted bool
}

var (
	servers sync.Map // uintptr => *Server
	nextID  atomic.Uintptr
)

// fileSystemHeader is the start of FSP_FILE_SYSTEM.
type fileSystemHeader struct {
	Version     uint16
	UserContext uintptr
}

// serverOf returns the Server from FSP_FILE_SYSTEM.UserContext.
func serverOf(fsp unsafe.Pointer) *Server {
	id := (*fileSystemHeader)(fsp).UserContext
	s, _ := servers.Load(id)
	return s.(*Server)
}

func (s *Server) file(fc uintptr) *file {
	return s.fs.handle(fc)
}

// MountPoint returns the mount point, eg. "X:".
func (s *Server) MountPoint() string { return s.mount }

// Wait waits until the file system is unmounted.
func (s *Server) Wait() { <-s.done }

// Unmount stops the dispatcher, and removes the mount point. Files
// that are still open are released. Unmounting again does nothing.
//
// Unmount always returns nil: the WinFsp calls it makes cannot fail.
// The error result matches fuse.Server.Unmount.
func (s *Server) Unmount() error {
	s.unmount.Do(func() {
		s.fs.setNotify(nil)
		s.api.fileSystemStopDisp.Call(uintptr(s.fsp))
		s.fspMu.Lock()
		s.deleted = true
		s.api.fileSystemDelete.Call(uintptr(s.fsp))
		s.fspMu.Unlock()
		servers.Delete(s.id)
		s.fs.shutdown()
		close(s.done)
	})
	return nil
}

// Mount mounts the fs tree at mountPoint, which is a drive letter
// ("X:"), a directory that does not exist yet, or "" to pick a free
// drive letter. See MountRaw for the options.
func Mount(mountPoint string, root fs.InodeEmbedder, options *fs.Options) (*Server, error) {
	raw := fs.NewNodeFS(root, options)
	var mo *fuse.MountOptions
	if options != nil {
		mo = &options.MountOptions
	}
	return MountRaw(mountPoint, raw, mo)
}

// MountRaw mounts a RawFileSystem. See Mount for mountPoint.
//
// Of opts, Name sets the file system name (default "go-fuse"), and
// FsName the volume label (default Name). Debug, Logger and
// PanicHandler work as for fuse.NewServer, and MaxWrite limits the
// size of reads and writes. Options for the kernel mount, such as
// AllowOther, Options and DirectMount, are ignored.
func MountRaw(mountPoint string, raw fuse.RawFileSystem, opts *fuse.MountOptions) (*Server, error) {
	a, err := loadAPI()
	if err != nil {
		return nil, err
	}
	initInterface()
	if opts == nil {
		opts = &fuse.MountOptions{}
	}

	s := &Server{api: a, done: make(chan struct{})}
	if err := s.initIdentity(); err != nil {
		return nil, err
	}
	caller := fuse.Caller{Owner: fuse.Owner{Uid: s.uid, Gid: s.gid}, Pid: uint32(os.Getpid())}
	s.fs, err = newFileSystem(raw, opts, caller)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	vp := volumeParams{
		Version:                  uint16(unsafe.Sizeof(volumeParams{})),
		SectorSize:               allocationUnit,
		SectorsPerAllocationUnit: 1,
		MaxComponentLength:       maxComponentLength,
		VolumeCreationTime:       filetime(uint64(now.Unix()), 0),
		VolumeSerialNumber:       uint32(now.Unix()),
		// Cache file info for 1s. WinFsp only caches file data
		// if this is infinite (FO_CACHE_SUPPORTED in
		// src/sys/create.c), so reads and writes always reach
		// the file system. To enable data caching, also set
		// Rsp.Create.Opened.DisableCache for FOPEN_DIRECT_IO
		// opens (fuseclient.Handle.Flags), as WinFsp's FUSE
		// layer does, and invalidate on NotifyContent.
		FileInfoTimeout: 1000,
		Flags: vfCaseSensitiveSearch | vfCasePreservedNames | vfUnicodeOnDisk |
			vfPersistentAcls | vfReparsePoints | vfPostCleanupWhenModifiedOnly |
			vfPassQueryDirectoryFileName | vfUmNoReparsePointsDirCheck |
			// One FileContext per handle (as WinFsp's FUSE layer does),
			// rather than one per open file.
			vfUmFileContextIsUserContext2,
	}
	name := opts.Name
	if name == "" {
		name = "go-fuse"
	}
	// Keep a terminating NUL.
	copy(vp.FileSystemName[:len(vp.FileSystemName)-1], utf16.Encode([]rune(name)))

	device, _ := windows.UTF16PtrFromString("WinFsp.Disk")
	var fsp unsafe.Pointer
	r, _, _ := a.fileSystemCreate.Call(uintptr(unsafe.Pointer(device)), uintptr(unsafe.Pointer(&vp)),
		uintptr(unsafe.Pointer(&iface[0])), uintptr(unsafe.Pointer(&fsp)))
	if ntStatus(r) != _STATUS_SUCCESS {
		return nil, fmt.Errorf("winfsp: FspFileSystemCreate: %w", ntStatus(r))
	}
	s.fsp = fsp

	s.id = nextID.Add(1)
	servers.Store(s.id, s)
	// A Go pointer may not be kept in WinFsp's memory, so store
	// an ID for the servers map.
	(*fileSystemHeader)(fsp).UserContext = s.id

	var mp *uint16
	if mountPoint != "" {
		mp, _ = windows.UTF16PtrFromString(mountPoint)
	}
	r, _, _ = a.fileSystemSetMountPoint.Call(uintptr(fsp), uintptr(unsafe.Pointer(mp)))
	if ntStatus(r) != _STATUS_SUCCESS {
		s.Unmount()
		return nil, fmt.Errorf("winfsp: FspFileSystemSetMountPoint(%q): %w", mountPoint, ntStatus(r))
	}
	r, _, _ = a.fileSystemMountPointF.Call(uintptr(fsp))
	// r points to memory owned by WinFsp.
	s.mount = windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&r)))

	r, _, _ = a.fileSystemStartDisp.Call(uintptr(fsp), 0)
	if ntStatus(r) != _STATUS_SUCCESS {
		s.Unmount()
		return nil, fmt.Errorf("winfsp: FspFileSystemStartDispatcher: %w", ntStatus(r))
	}
	s.fs.setNotify(s.notify)
	return s, nil
}

// initIdentity maps the process user and group to POSIX ids. Files are
// reported as owned by this user.
func (s *Server) initIdentity() error {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return fmt.Errorf("winfsp: GetTokenUser: %w", err)
	}
	g, err := tok.GetTokenPrimaryGroup()
	if err != nil {
		return fmt.Errorf("winfsp: GetTokenPrimaryGroup: %w", err)
	}
	if r, _, _ := s.api.posixMapSidToUid.Call(uintptr(unsafe.Pointer(u.User.Sid)), uintptr(unsafe.Pointer(&s.uid))); ntStatus(r) != _STATUS_SUCCESS {
		return fmt.Errorf("winfsp: FspPosixMapSidToUid: %w", ntStatus(r))
	}
	if r, _, _ := s.api.posixMapSidToUid.Call(uintptr(unsafe.Pointer(g.PrimaryGroup)), uintptr(unsafe.Pointer(&s.gid))); ntStatus(r) != _STATUS_SUCCESS {
		return fmt.Errorf("winfsp: FspPosixMapSidToUid: %w", ntStatus(r))
	}
	return nil
}

// FSP_FSCTL_NOTIFY_INFO filter and action values.
const (
	fileNotifyChangeFileName  = 0x1
	fileNotifyChangeDirName   = 0x2
	fileNotifyChangeAttribute = 0x4
	fileNotifyChangeSize      = 0x8
	fileNotifyChangeLastWrite = 0x10

	fileActionRemoved  = 2
	fileActionModified = 3
)

// notifyInfo mirrors FSP_FSCTL_NOTIFY_INFO, without the file name that
// follows it.
type notifyInfo struct {
	Size   uint16
	Filter uint32
	Action uint32
}

var _ [12]byte = [unsafe.Sizeof(notifyInfo{})]byte{}

func (s *Server) notify(path string, c change) {
	u := utf16.Encode([]rune(path))
	hdr := int(unsafe.Sizeof(notifyInfo{}))
	size := hdr + 2*len(u)
	buf := alignedBuf(size)
	ni := (*notifyInfo)(unsafe.Pointer(&buf[0]))
	ni.Size = uint16(size)
	const names = fileNotifyChangeFileName | fileNotifyChangeDirName
	const contents = fileNotifyChangeAttribute | fileNotifyChangeSize | fileNotifyChangeLastWrite
	switch c {
	case changeInode:
		ni.Filter = contents
		ni.Action = fileActionModified
	case changeEntry:
		// The entry may be new, renamed or gone; we do not
		// know which, so tell watchers of names too.
		ni.Filter = names | contents
		ni.Action = fileActionModified
	case changeRemoved:
		ni.Filter = names
		ni.Action = fileActionRemoved
	}
	copy(utf16At(buf, hdr, len(u)), u)
	s.fspMu.RLock()
	defer s.fspMu.RUnlock()
	if s.deleted {
		return
	}
	r, _, _ := s.api.fileSystemNotifyBegin.Call(uintptr(s.fsp), 1000)
	if ntStatus(r) != _STATUS_SUCCESS {
		// TODO: the notification is dropped without a trace,
		// eg. if NotifyBegin times out waiting for the rename
		// lock. WinFsp's caches still expire after
		// FileInfoTimeout, but Windows misses the change
		// notification. Log it with Debug, or retry later.
		return
	}
	s.api.fileSystemNotify.Call(uintptr(s.fsp), uintptr(unsafe.Pointer(&buf[0])), uintptr(size))
	s.api.fileSystemNotifyEnd.Call(uintptr(s.fsp))
}

func putFileInfoPtr(dst unsafe.Pointer, fi *fileInfo) {
	if dst != nil {
		*(*fileInfo)(dst) = *fi
	}
}

func ret(st ntStatus) uintptr { return uintptr(st) }

// securityDescriptor returns a self-relative security descriptor for
// the given attributes, in memory owned by WinFsp, and a function to
// free it.
func (s *Server) securityDescriptor(a *fuse.Attr) (unsafe.Pointer, func(), ntStatus) {
	var sd unsafe.Pointer
	mode := a.Mode & 07777
	r, _, _ := s.api.posixMapPermsToSD.Call(uintptr(s.uid), uintptr(s.gid), uintptr(mode), uintptr(unsafe.Pointer(&sd)))
	if ntStatus(r) != _STATUS_SUCCESS {
		return nil, nil, ntStatus(r)
	}
	return sd, func() {
		s.api.deleteSecurityDesc.Call(uintptr(sd), s.api.posixMapPermsToSD.Addr())
	}, _STATUS_SUCCESS
}

// copySecurityDescriptor implements the PSECURITY_DESCRIPTOR /
// SIZE_T* output convention.
func (s *Server) copySecurityDescriptor(a *fuse.Attr, dst unsafe.Pointer, psize *uintptr) ntStatus {
	if psize == nil {
		return _STATUS_SUCCESS
	}
	sd, free, st := s.securityDescriptor(a)
	if st != _STATUS_SUCCESS {
		return st
	}
	defer free()
	n := uintptr((*windows.SECURITY_DESCRIPTOR)(sd).Length())
	if n > *psize {
		*psize = n
		return _STATUS_BUFFER_OVERFLOW
	}
	*psize = n
	if dst != nil {
		copy(unsafe.Slice((*byte)(dst), n), unsafe.Slice((*byte)(sd), n))
	}
	return _STATUS_SUCCESS
}

// modeFromSD returns the permission bits of a security descriptor.
func (s *Server) modeFromSD(sd unsafe.Pointer) (uint32, ntStatus) {
	var uid, gid, mode uint32
	r, _, _ := s.api.posixMapSDToPerms.Call(uintptr(sd), uintptr(unsafe.Pointer(&uid)), uintptr(unsafe.Pointer(&gid)), uintptr(unsafe.Pointer(&mode)))
	return mode, ntStatus(r)
}

func cbGetVolumeInfo(fsp unsafe.Pointer, vi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	info, st := s.fs.getVolumeInfo()
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	putVolumeInfo(vi, &info)
	return ret(_STATUS_SUCCESS)
}

// fspVolumeInfo mirrors FSP_FSCTL_VOLUME_INFO.
type fspVolumeInfo struct {
	TotalSize         uint64
	FreeSize          uint64
	VolumeLabelLength uint16 // in bytes
	VolumeLabel       [32]uint16
}

// fspVolumeInfo must have the layout of FSP_FSCTL_VOLUME_INFO, whose
// label ends at byte 82.
var _ [82]byte = [unsafe.Offsetof(fspVolumeInfo{}.VolumeLabel) + unsafe.Sizeof(fspVolumeInfo{}.VolumeLabel)]byte{}

func putVolumeInfo(vi unsafe.Pointer, info *volumeInfo) {
	out := (*fspVolumeInfo)(vi)
	*out = fspVolumeInfo{
		TotalSize: info.TotalSize,
		FreeSize:  info.FreeSize,
	}
	n := copy(out.VolumeLabel[:], utf16.Encode([]rune(info.Label)))
	out.VolumeLabelLength = uint16(2 * n)
}

func cbSetVolumeLabel(fsp unsafe.Pointer, label *uint16, vi unsafe.Pointer) uintptr {
	return cbGetVolumeInfo(fsp, vi)
}

func cbGetSecurityByName(fsp unsafe.Pointer, name *uint16, pattrs *uint32, sd unsafe.Pointer, psize *uintptr) uintptr {
	s := serverOf(fsp)
	path := windows.UTF16PtrToString(name)
	attrs, a, st := s.fs.getSecurityByName(path)
	if st == _STATUS_OBJECT_PATH_NOT_FOUND || st == _STATUS_OBJECT_NAME_NOT_FOUND {
		// A symlink in the middle of the path?
		var idx uint32
		r, _, _ := s.api.findReparsePoint.Call(uintptr(fsp), getReparsePointByNameCB, 0,
			uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&idx)))
		if byte(r) != 0 {
			if pattrs != nil {
				*pattrs = idx
			}
			return ret(_STATUS_REPARSE)
		}
	}
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	if pattrs != nil {
		*pattrs = attrs
	}
	return ret(s.copySecurityDescriptor(&a, sd, psize))
}

func (s *Server) createMode(sd unsafe.Pointer, dir bool) uint32 {
	mode := uint32(0666)
	if dir {
		mode = 0777
	}
	if sd != nil {
		if m, st := s.modeFromSD(sd); st == _STATUS_SUCCESS {
			mode = m
		}
	}
	return mode &^ 022
}

// cbCreate ignores allocationSize: FUSE has no allocation size (see
// setFileSize). The new file is opened read-write, whatever
// grantedAccess is, as WinFsp's FUSE layer does.
func cbCreate(fsp unsafe.Pointer, name *uint16, createOptions, grantedAccess, fileAttributes uint32,
	sd unsafe.Pointer, allocationSize uint64, pfc *uintptr, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	mode := s.createMode(sd, createOptions&_FILE_DIRECTORY_FILE != 0)
	f, info, st := s.fs.create(windows.UTF16PtrToString(name), createOptions, fileAttributes, mode)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	*pfc = s.fs.addHandle(f)
	putFileInfoPtr(fi, &info)
	return ret(_STATUS_SUCCESS)
}

func cbOpen(fsp unsafe.Pointer, name *uint16, createOptions, grantedAccess uint32, pfc *uintptr, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f, info, st := s.fs.open(windows.UTF16PtrToString(name), grantedAccess)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	*pfc = s.fs.addHandle(f)
	putFileInfoPtr(fi, &info)
	return ret(_STATUS_SUCCESS)
}

func cbOverwrite(fsp unsafe.Pointer, fc uintptr, fileAttributes uint32, replace uint8, allocationSize uint64, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	info, st := s.fs.overwrite(f, fileAttributes, replace != 0)
	if st == _STATUS_SUCCESS {
		putFileInfoPtr(fi, &info)
	}
	return ret(st)
}

func cbCleanup(fsp unsafe.Pointer, fc uintptr, name *uint16, flags uint32) uintptr {
	s := serverOf(fsp)
	if f := s.file(fc); f != nil {
		s.fs.cleanup(f, windows.UTF16PtrToString(name), flags)
	}
	return 0
}

func cbClose(fsp unsafe.Pointer, fc uintptr) uintptr {
	s := serverOf(fsp)
	s.fs.closeHandle(fc)
	return 0
}

func cbRead(fsp unsafe.Pointer, fc uintptr, buf unsafe.Pointer, off uint64, length uint32, pn *uint32) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	n, st := s.fs.read(f, unsafe.Slice((*byte)(buf), length), off)
	*pn = uint32(n)
	return ret(st)
}

func cbWrite(fsp unsafe.Pointer, fc uintptr, buf unsafe.Pointer, off uint64, length uint32,
	toEnd, constrained uint8, pn *uint32, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	n, info, st := s.fs.write(f, unsafe.Slice((*byte)(buf), length), off, toEnd != 0, constrained != 0)
	*pn = uint32(n)
	if st == _STATUS_SUCCESS {
		putFileInfoPtr(fi, &info)
	}
	return ret(st)
}

func cbFlush(fsp unsafe.Pointer, fc uintptr, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	var f *file
	if fc != 0 {
		f = s.file(fc)
	}
	info, st := s.fs.flush(f)
	if st == _STATUS_SUCCESS && f != nil {
		putFileInfoPtr(fi, &info)
	}
	return ret(st)
}

func cbGetFileInfo(fsp unsafe.Pointer, fc uintptr, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	info, st := s.fs.getFileInfo(f)
	if st == _STATUS_SUCCESS {
		putFileInfoPtr(fi, &info)
	}
	return ret(st)
}

func cbSetBasicInfo(fsp unsafe.Pointer, fc uintptr, attrs uint32, ctime, atime, mtime, chtime uint64, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	info, st := s.fs.setBasicInfo(f, attrs, ctime, atime, mtime, chtime)
	if st == _STATUS_SUCCESS {
		putFileInfoPtr(fi, &info)
	}
	return ret(st)
}

func cbSetFileSize(fsp unsafe.Pointer, fc uintptr, size uint64, alloc uint8, fi unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	info, st := s.fs.setFileSize(f, size, alloc != 0)
	if st == _STATUS_SUCCESS {
		putFileInfoPtr(fi, &info)
	}
	return ret(st)
}

func cbCanDelete(fsp unsafe.Pointer, fc uintptr, name *uint16) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	return ret(s.fs.canDelete(f))
}

func cbRename(fsp unsafe.Pointer, fc uintptr, name, newName *uint16, replace uint8) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	return ret(s.fs.rename(windows.UTF16PtrToString(name), windows.UTF16PtrToString(newName), replace != 0))
}

func cbGetSecurity(fsp unsafe.Pointer, fc uintptr, sd unsafe.Pointer, psize *uintptr) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	a, st := s.fs.getSecurity(f)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	return ret(s.copySecurityDescriptor(&a, sd, psize))
}

func cbSetSecurity(fsp unsafe.Pointer, fc uintptr, info uint32, modification unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	a, st := s.fs.getSecurity(f)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	cur, free, st := s.securityDescriptor(&a)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	defer free()
	var newSD unsafe.Pointer
	r, _, _ := s.api.setSecurityDescriptor.Call(uintptr(cur), uintptr(info), uintptr(modification), uintptr(unsafe.Pointer(&newSD)))
	if ntStatus(r) != _STATUS_SUCCESS {
		return r
	}
	defer s.api.deleteSecurityDesc.Call(uintptr(newSD), s.api.setSecurityDescriptor.Addr())
	mode, st := s.modeFromSD(newSD)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	return ret(s.fs.setSecurity(f, mode, nil, nil))
}

func cbReadDirectory(fsp unsafe.Pointer, fc uintptr, pattern, marker *uint16, buf unsafe.Pointer, length uint32, pn *uint32) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	var m *string
	if marker != nil {
		str := windows.UTF16PtrToString(marker)
		m = &str
	}
	n, st := s.fs.readDirectory(f, m, unsafe.Slice((*byte)(buf), length))
	*pn = uint32(n)
	return ret(st)
}

// dirInfoByNameSize is the size of the buffer that WinFsp passes to
// GetDirInfoByName (FspFileSystemOpQueryDirectory_GetDirInfoByName in
// src/dll/fsop.c): a union of FSP_FSCTL_DIR_INFO and
// UINT8[sizeof(FSP_FSCTL_DIR_INFO) + 255 * sizeof(WCHAR)]. The size of
// a union is rounded up to its alignment, which is that of
// FSP_FSCTL_DIR_INFO.
const dirInfoByNameSize = (dirInfoSize + 255*2 + dirInfoAlign - 1) / dirInfoAlign * dirInfoAlign

const dirInfoAlign = int(unsafe.Alignof(dirInfo{}))

var _ [616]byte = [dirInfoByNameSize]byte{}

func cbGetDirInfoByName(fsp unsafe.Pointer, fc uintptr, name *uint16, di unsafe.Pointer) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	str := windows.UTF16PtrToString(name)
	info, st := s.fs.getDirInfoByName(f, str)
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	buf := unsafe.Slice((*byte)(di), dirInfoByNameSize)
	if _, ok := appendDirInfo(buf, 0, str, &info); !ok {
		return ret(_STATUS_OBJECT_NAME_INVALID)
	}
	return ret(_STATUS_SUCCESS)
}

// Symlinks are reported as IO_REPARSE_TAG_SYMLINK reparse points.

func copyReparse(data []byte, buf unsafe.Pointer, psize *uintptr) ntStatus {
	if uintptr(len(data)) > *psize {
		return _STATUS_BUFFER_TOO_SMALL
	}
	*psize = uintptr(len(data))
	if buf != nil {
		copy(unsafe.Slice((*byte)(buf), len(data)), data)
	}
	return _STATUS_SUCCESS
}

func (s *Server) getReparsePoint(path string, buf unsafe.Pointer, psize *uintptr) ntStatus {
	target, st := s.fs.readlink(path)
	if st != _STATUS_SUCCESS {
		return st
	}
	data, st := symlinkReparseData(target)
	if st != _STATUS_SUCCESS {
		return st
	}
	return copyReparse(data, buf, psize)
}

func cbGetReparsePointByName(fsp unsafe.Pointer, ctx uintptr, name *uint16, isDir uint8, buf unsafe.Pointer, psize *uintptr) uintptr {
	s := serverOf(fsp)
	path := windows.UTF16PtrToString(name)
	if psize == nil {
		_, st := s.fs.readlink(path)
		return ret(st)
	}
	return ret(s.getReparsePoint(path, buf, psize))
}

func cbGetReparsePoint(fsp unsafe.Pointer, fc uintptr, name *uint16, buf unsafe.Pointer, psize *uintptr) uintptr {
	s := serverOf(fsp)
	return ret(s.getReparsePoint(windows.UTF16PtrToString(name), buf, psize))
}

func cbSetReparsePoint(fsp unsafe.Pointer, fc uintptr, name *uint16, buf unsafe.Pointer, size uintptr) uintptr {
	s := serverOf(fsp)
	f := s.file(fc)
	if f == nil {
		return ret(_STATUS_INVALID_HANDLE)
	}
	target, st := parseSymlinkReparseData(unsafe.Slice((*byte)(buf), size))
	if st != _STATUS_SUCCESS {
		return ret(st)
	}
	return ret(s.fs.setSymlink(f, windows.UTF16PtrToString(name), target))
}

// cbDispatcherStopped is called last on the dispatcher thread. If
// WinFsp unmounted the volume by itself (normally == 0), eg. because
// the driver is unloaded, clean up as Unmount does.
func cbDispatcherStopped(fsp unsafe.Pointer, normally uint8) uintptr {
	if normally == 0 {
		// Unmount waits for this thread to exit.
		go serverOf(fsp).Unmount()
	}
	return 0
}

func cbResolveReparsePoints(fsp unsafe.Pointer, name *uint16, idx uint32, resolveLast uint8, iosb unsafe.Pointer, buf unsafe.Pointer, psize *uintptr) uintptr {
	s := serverOf(fsp)
	b := uintptr(0)
	if resolveLast != 0 {
		b = 1
	}
	r, _, _ := s.api.resolveReparsePoints.Call(uintptr(fsp), getReparsePointByNameCB, 0,
		uintptr(unsafe.Pointer(name)), uintptr(idx), b, uintptr(iosb), uintptr(buf), uintptr(unsafe.Pointer(psize)))
	return r
}
