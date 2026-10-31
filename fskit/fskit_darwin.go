// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

// Package fskit implements a FUSE file system frontend on macOS using the
// macFUSE 5.3.0+ FSKit user-space framework and fuse.ProtocolServer.
package fskit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/iov"
)

// mfmountFrameworkPath is the canonical path to the macFUSE MFMount framework
// binary installed by macFUSE 5.3.0+.
const mfmountFrameworkPath = "/Library/Filesystems/macfuse.fs/Contents/Frameworks/MFMount.framework/MFMount"

// MFMountResult enum values from MFMount.h.
const (
	mfMountResultSuccess                             int32 = 0
	mfMountResultUnsupportedOSVersion                int32 = 1
	mfMountResultHelperToolsInstallationFailed       int32 = 2
	mfMountResultFileSystemExtensionNotFound         int32 = 3
	mfMountResultFileSystemExtensionRequiresApproval int32 = 4
	mfMountResultCreatingMountPointFailed            int32 = 5
	mfMountResultUnexpectedFailure                   int32 = -1
)

type mfMountError int32

func (e mfMountError) Error() string {
	switch int32(e) {
	case mfMountResultUnsupportedOSVersion:
		return "fskit: unsupported version of macOS"
	case mfMountResultHelperToolsInstallationFailed:
		return "fskit: macFUSE helper tools not installed; open the macFUSE app once to install them"
	case mfMountResultFileSystemExtensionNotFound:
		return "fskit: macFUSE FSKit extension not registered; open /Library/Filesystems/macfuse.fs/Contents/Resources/macfuse.app once to register it"
	case mfMountResultFileSystemExtensionRequiresApproval:
		return "fskit: macFUSE FSKit extension is not approved; enable it in System Settings → General → Login Items & Extensions → File System Extensions"
	case mfMountResultCreatingMountPointFailed:
		return "fskit: creating mount point failed"
	case mfMountResultUnexpectedFailure:
		return "fskit: unexpected failure"
	default:
		return fmt.Sprintf("fskit: mount failed (rc=%d)", int32(e))
	}
}

func mfMountErr(rc int32) error {
	if rc == mfMountResultSuccess {
		return nil
	}
	return mfMountError(rc)
}

// darwinIovec matches <sys/uio.h> on 64-bit Darwin.
type darwinIovec struct {
	base unsafe.Pointer
	len  uint64
}

// C function types for MFMount.framework symbols.
type (
	fnMFChannelCreate         func() uintptr
	fnMFMount                 func(channel uintptr, mountPoint, options string, quiet bool) int32
	fnMFMessageGetBodySize    func(message uintptr) int
	fnMFMessageGetBodyBuffers func(message uintptr, buffers *unsafe.Pointer) int
	fnMFChannelInterrupt      func(channel uintptr) bool
	fnMFChannelClose          func(channel uintptr) bool
	fnMFRetain                func(ref uintptr) uintptr
	fnMFRelease               func(ref uintptr)
)

type mfMountAPI struct {
	channelCreate         fnMFChannelCreate
	mount                 fnMFMount
	symCopyNextMessage    uintptr
	messageGetBodySize    fnMFMessageGetBodySize
	messageGetBodyBuffers fnMFMessageGetBodyBuffers
	symSendMessage        uintptr
	channelInterrupt      fnMFChannelInterrupt
	channelClose          fnMFChannelClose
	retain                fnMFRetain
	release               fnMFRelease
}

var (
	mfAPIOnce sync.Once
	mfAPI     *mfMountAPI
	mfAPIErr  error
)

func loadMFMountAPI() (api *mfMountAPI, err error) {
	h, err := purego.Dlopen(mfmountFrameworkPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("fskit: cannot load %s: %w", mfmountFrameworkPath, err)
	}

	api = &mfMountAPI{}
	defer func() {
		if r := recover(); r != nil {
			api = nil
			err = fmt.Errorf("fskit: missing symbol in %s: %v", mfmountFrameworkPath, r)
		}
	}()

	symCopy, err := purego.Dlsym(h, "MFChannelCopyNextMessage")
	if err != nil {
		return nil, fmt.Errorf("fskit: missing MFChannelCopyNextMessage: %w", err)
	}
	api.symCopyNextMessage = symCopy

	symSend, err := purego.Dlsym(h, "MFChannelSendMessage")
	if err != nil {
		return nil, fmt.Errorf("fskit: missing MFChannelSendMessage: %w", err)
	}
	api.symSendMessage = symSend

	purego.RegisterLibFunc(&api.channelCreate, h, "MFChannelCreate")
	purego.RegisterLibFunc(&api.mount, h, "MFMount")
	purego.RegisterLibFunc(&api.messageGetBodySize, h, "MFMessageGetBodySize")
	purego.RegisterLibFunc(&api.messageGetBodyBuffers, h, "MFMessageGetBodyBuffers")
	purego.RegisterLibFunc(&api.channelInterrupt, h, "MFChannelInterrupt")
	purego.RegisterLibFunc(&api.channelClose, h, "MFChannelClose")
	purego.RegisterLibFunc(&api.retain, h, "MFRetain")
	purego.RegisterLibFunc(&api.release, h, "MFRelease")

	return api, nil
}

func getMFMountAPI() (*mfMountAPI, error) {
	mfAPIOnce.Do(func() {
		mfAPI, mfAPIErr = loadMFMountAPI()
	})
	return mfAPI, mfAPIErr
}

// Available reports whether the macFUSE FSKit framework is installed and usable
// on the current system.
func Available() bool {
	_, err := getMFMountAPI()
	return err == nil
}

// appleXattrPrefix is the prefix shared by Apple system xattrs that FSKit's
// create flow expects to succeed.
var appleXattrPrefix = []byte("com.apple.")

const (
	opSetXAttr         = uint32(21)
	sizeOfInHeader     = 40
	sizeOfOutHeader    = 16
	setXAttrHeaderSize = 56 // InHeader(40) + Size(4) + Flags(4) + Position(4) + Padding(4)
)

// isAppleSetXAttr reports whether in is a SETXATTR for a com.apple.* attribute.
// FSKit treats SETXATTR failure on these as fatal to file creation.
func isAppleSetXAttr(in [][]byte) bool {
	if len(in) == 0 || len(in[0]) < sizeOfInHeader {
		return false
	}
	op := binary.LittleEndian.Uint32(in[0][4:8])
	if op != opSetXAttr {
		return false
	}
	return iov.HasPrefixAt(in, setXAttrHeaderSize, appleXattrPrefix)
}

type responseBuffer struct {
	hdr     [sizeOfOutHeader]byte
	payload []byte
	iov     [][]byte
}

// Server represents an active FSKit file system mount session.
type Server struct {
	rawFS      fuse.RawFileSystem
	ps         *fuse.ProtocolServer
	mfChannel  uintptr
	api        *mfMountAPI
	mountPoint string
	opts       *fuse.MountOptions

	responsePool sync.Pool

	closed     atomic.Bool
	mounted    chan error
	serverDone chan struct{}
	handlersWg sync.WaitGroup
}

// Mount mounts rawFS on mountPoint using macOS FSKit (macFUSE 5.3.0+).
// Mount blocks until the file system is successfully mounted and verified by
// macOS, or returns an error.
//
// FUSE notifications (e.g. EntryNotify, InodeNotify) are not supported by the
// macFUSE FSKit backend and return ENOSYS.
func Mount(mountPoint string, rawFS fuse.RawFileSystem, opts *fuse.MountOptions) (*Server, error) {
	absMountPoint, err := filepath.Abs(mountPoint)
	if err != nil {
		return nil, err
	}

	api, err := getMFMountAPI()
	if err != nil {
		return nil, err
	}

	channel := api.channelCreate()
	if channel == 0 {
		return nil, errors.New("fskit: MFChannelCreate failed")
	}

	var optsCopy fuse.MountOptions
	if opts != nil {
		optsCopy = *opts
	}
	if optsCopy.Logger == nil {
		optsCopy.Logger = log.Default()
	}
	optsCopy.DisableSplice = true

	maxPayload := max(optsCopy.MaxWrite, 1<<20)
	s := &Server{
		rawFS:      rawFS,
		ps:         fuse.NewProtocolServer(rawFS, &optsCopy),
		mfChannel:  channel,
		api:        api,
		mountPoint: absMountPoint,
		opts:       &optsCopy,
		mounted:    make(chan error, 1),
		serverDone: make(chan struct{}),
	}

	s.responsePool = sync.Pool{
		New: func() any {
			rb := &responseBuffer{
				payload: make([]byte, maxPayload),
			}
			rb.iov = [][]byte{rb.hdr[:], rb.payload}
			return rb
		},
	}

	go s.serve()

	channelRetained := api.retain(channel)
	optsStr := strings.Join(formatOptions(&optsCopy), ",")

	go func() {
		rc := api.mount(channelRetained, absMountPoint, optsStr, true)
		api.release(channelRetained)
		s.mounted <- mfMountErr(rc)
		close(s.mounted)
	}()

	if mountErr := <-s.mounted; mountErr != nil {
		if err := s.Unmount(); err != nil && s.opts.Debug {
			s.opts.Logger.Printf("fskit: unmount after mount failure: %v", err)
		}
		return nil, mountErr
	}

	return s, nil
}

// MountFS mounts a node filesystem on dir using macOS FSKit.
func MountFS(dir string, root fs.InodeEmbedder, options *fs.Options) (*Server, error) {
	rawFS := fs.NewNodeFS(root, options)
	var mountOptions *fuse.MountOptions
	if options != nil {
		mountOptions = &options.MountOptions
	}
	return Mount(dir, rawFS, mountOptions)
}

// Wait blocks until the filesystem is unmounted.
func (s *Server) Wait() {
	<-s.serverDone
}

func (s *Server) closeChannel() {
	if s.closed.CompareAndSwap(false, true) {
		s.api.channelInterrupt(s.mfChannel)
		s.api.channelClose(s.mfChannel)
		s.api.release(s.mfChannel)
	}
}

// mntForce is MNT_FORCE from Darwin <sys/mount.h>.
const mntForce = 0x00080000

// Unmount unmounts the filesystem and terminates the server session.
func (s *Server) Unmount() error {
	s.closeChannel()

	err := syscall.Unmount(s.mountPoint, 0)
	if err != nil {
		err = syscall.Unmount(s.mountPoint, mntForce)
	}

	<-s.serverDone
	return err
}

func (s *Server) serve() {
	defer func() {
		s.handlersWg.Wait()
		s.closeChannel()
		close(s.serverDone)
	}()

	for {
		if s.closed.Load() {
			return
		}

		msg, _, rawErr := purego.SyscallN(s.api.symCopyNextMessage, s.mfChannel)
		if msg == 0 {
			errNo := syscall.Errno(rawErr)
			if errNo == syscall.ENOENT || errNo == syscall.EINTR || errNo == syscall.EAGAIN {
				continue
			}
			return
		}

		s.handlersWg.Add(1)
		go func(m uintptr) {
			defer s.handlersWg.Done()
			s.handle(m)
		}(msg)
	}
}

func (s *Server) handle(msg uintptr) {
	defer s.api.release(msg)

	bodySize := s.api.messageGetBodySize(msg)
	if bodySize < 0 {
		return
	}

	var bufsPtr unsafe.Pointer
	bodyCount := s.api.messageGetBodyBuffers(msg, &bufsPtr)
	if bodyCount < 1 || bufsPtr == nil {
		return
	}

	iovs := unsafe.Slice((*darwinIovec)(bufsPtr), bodyCount)
	var inStack [4][]byte
	var in [][]byte
	if bodyCount <= len(inStack) {
		in = inStack[:bodyCount]
	} else {
		in = make([][]byte, bodyCount)
	}

	for i := 0; i < bodyCount; i++ {
		in[i] = unsafe.Slice((*byte)(iovs[i].base), int(iovs[i].len))
	}

	maskError := isAppleSetXAttr(in)

	rb := s.responsePool.Get().(*responseBuffer)
	defer s.responsePool.Put(rb)
	clear(rb.hdr[:])

	n, _ := s.ps.HandleRequest(in, rb.iov)
	if n <= 0 {
		return
	}

	if maskError && int32(binary.LittleEndian.Uint32(rb.hdr[4:8])) < 0 {
		binary.LittleEndian.PutUint32(rb.hdr[4:8], 0)
	}

	s.sendMessage(rb.hdr[:], rb.payload, n)
}

func (s *Server) sendMessage(outHdr, outPayload []byte, n int) {
	if s.closed.Load() {
		return
	}

	var stackIovs [2]darwinIovec
	diovs := stackIovs[:0]

	hdrLen := min(n, sizeOfOutHeader)
	diovs = append(diovs, darwinIovec{
		base: unsafe.Pointer(&outHdr[0]),
		len:  uint64(hdrLen),
	})

	if n > sizeOfOutHeader {
		payloadLen := n - sizeOfOutHeader
		diovs = append(diovs, darwinIovec{
			base: unsafe.Pointer(&outPayload[0]),
			len:  uint64(payloadLen),
		})
	}

	r1, _, rawErr := purego.SyscallN(s.api.symSendMessage, s.mfChannel, uintptr(unsafe.Pointer(&diovs[0])), uintptr(len(diovs)))
	res := int(r1)
	if res < 0 {
		errNo := syscall.Errno(rawErr)
		if errNo == syscall.ENOENT {
			return
		}
		if s.opts.Debug {
			s.opts.Logger.Printf("fskit: MFChannelSendMessage error: %v", errNo)
		}
	}
}

func formatOptions(o *fuse.MountOptions) []string {
	var r []string
	r = append(r, o.Options...)

	if o.AllowOther {
		r = append(r, "allow_other")
	}
	if o.FsName != "" {
		r = append(r, "fsname="+o.FsName)
	}
	if o.Name != "" {
		r = append(r, "subtype="+o.Name)
	}
	if o.MaxWrite > 0 {
		r = append(r, fmt.Sprintf("max_read=%d", o.MaxWrite))
		r = append(r, fmt.Sprintf("iosize=%d", o.MaxWrite))
	}
	r = append(r, "daemon_timeout=0")

	var rEscaped []string
	for _, s := range r {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `,`, `\,`)
		rEscaped = append(rEscaped, s)
	}
	return rEscaped
}
