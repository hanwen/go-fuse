// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package fuseclient

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v3/fuse"
)

// FUSE opcodes, from include/uapi/linux/fuse.h.
const (
	opLookup      = 1
	opForget      = 2
	opGetattr     = 3
	opSetattr     = 4
	opReadlink    = 5
	opSymlink     = 6
	opMkdir       = 9
	opUnlink      = 10
	opRmdir       = 11
	opRename      = 12
	opOpen        = 14
	opRead        = 15
	opWrite       = 16
	opStatfs      = 17
	opRelease     = 18
	opFsync       = 20
	opFlush       = 25
	opInit        = 26
	opOpendir     = 27
	opReaddir     = 28
	opReleasedir  = 29
	opCreate      = 35
	opReaddirplus = 44
)

// minor is the protocol minor version that the client announces.
const minor = 45

// Client sends FUSE requests to a fuse.ProtocolServer in-process.
// Its methods are safe for concurrent use.
type Client struct {
	ps     *fuse.ProtocolServer
	unique atomic.Uint64
	init   fuse.InitOut
}

// New creates a ProtocolServer for fs, and sends INIT to it.
// Notify receives the notifications from the file system.
func New(fs fuse.RawFileSystem, opts *fuse.MountOptions, notify fuse.Notify) (*Client, error) {
	if opts == nil {
		opts = &fuse.MountOptions{}
	}
	c := &Client{
		ps: fuse.NewProtocolServer(fs, opts, &notify),
	}

	in := fuse.InitIn{
		Major:        7,
		Minor:        minor,
		MaxReadAhead: 128 * 1024,
	}
	flags := uint64(fuse.CAP_ASYNC_READ | fuse.CAP_BIG_WRITES | fuse.CAP_READDIRPLUS |
		fuse.CAP_PARALLEL_DIROPS | fuse.CAP_MAX_PAGES)
	in.Flags = uint32(flags)
	in.Flags2 = uint32(flags >> 32)
	errno := c.do(opInit, 0, nil, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, unsafe.Pointer(&c.init), unsafe.Sizeof(c.init), nil)
	if errno != 0 {
		return nil, fmt.Errorf("fuseclient: INIT: %v", errno)
	}
	if c.init.MaxWrite == 0 {
		c.init.MaxWrite = 128 * 1024
	}
	return c, nil
}

// InitOut returns the INIT reply.
func (c *Client) InitOut() fuse.InitOut {
	return c.init
}

// MaxWrite is the largest READ or WRITE that the client sends.
func (c *Client) MaxWrite() int {
	return int(c.init.MaxWrite)
}

// do sends one request. in points to a struct starting with
// fuse.InHeader, of size inSize. The reply struct goes to out
// (outSize bytes), and the reply payload to outPayload. It returns
// the payload size.
func (c *Client) do(op uint32, node uint64, caller *fuse.Caller,
	in unsafe.Pointer, inSize uintptr, inPayload []byte,
	out unsafe.Pointer, outSize uintptr, outPayload []byte) syscall.Errno {
	_, errno := c.doN(op, node, caller, in, inSize, inPayload, out, outSize, outPayload)
	return errno
}

func (c *Client) doN(op uint32, node uint64, caller *fuse.Caller,
	in unsafe.Pointer, inSize uintptr, inPayload []byte,
	out unsafe.Pointer, outSize uintptr, outPayload []byte) (int, syscall.Errno) {
	h := (*fuse.InHeader)(in)
	h.Opcode = op
	h.Unique = c.unique.Add(1)
	h.NodeId = node
	h.Length = uint32(inSize) + uint32(len(inPayload))
	if caller != nil {
		h.Caller = *caller
	}

	inIov := [][]byte{unsafe.Slice((*byte)(in), inSize)}
	if len(inPayload) > 0 {
		inIov = append(inIov, inPayload)
	}

	var hdr [16]byte
	outIov := [][]byte{hdr[:]}
	if outSize > 0 {
		outIov = append(outIov, unsafe.Slice((*byte)(out), outSize))
	}
	if len(outPayload) > 0 {
		outIov = append(outIov, outPayload)
	}

	n, st := c.ps.HandleRequest(inIov, outIov)
	if st != 0 {
		return 0, st
	}
	if op == opForget {
		return 0, 0
	}
	if n < len(hdr) {
		return 0, syscall.EIO
	}
	if code := int32(binary.LittleEndian.Uint32(hdr[4:])); code != 0 {
		return 0, syscall.Errno(-code)
	}
	payload := n - len(hdr) - int(outSize)
	if payload < 0 {
		// Short replies are allowed for some structs; the
		// remainder stays zero.
		payload = 0
	}
	return payload, 0
}

// cstr returns the strings as a request payload, each terminated by
// a NUL byte. It fails with EINVAL if a string contains a NUL byte,
// which would end it early.
func cstr(strs ...string) ([]byte, syscall.Errno) {
	var b []byte
	for _, s := range strs {
		if strings.IndexByte(s, 0) >= 0 {
			return nil, syscall.EINVAL
		}
		b = append(b, s...)
		b = append(b, 0)
	}
	return b, 0
}

// Lookup sends LOOKUP. A successful reply with NodeId == 0 is a
// negative entry, and is returned as ENOENT with the entry.
func (c *Client) Lookup(caller *fuse.Caller, parent uint64, name string) (fuse.EntryOut, syscall.Errno) {
	var in fuse.InHeader
	var out fuse.EntryOut
	payload, errno := cstr(name)
	if errno != 0 {
		return out, errno
	}
	errno = c.do(opLookup, parent, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), payload,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	if errno == 0 && out.NodeId == 0 {
		errno = syscall.ENOENT
	}
	return out, errno
}

// Forget sends FORGET.
func (c *Client) Forget(node uint64, nlookup uint64) {
	in := fuse.ForgetIn{Nlookup: nlookup}
	c.do(opForget, node, nil, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, nil)
}

// Getattr sends GETATTR, with the file handle if fh is non-nil.
func (c *Client) Getattr(caller *fuse.Caller, node uint64, fh *uint64) (fuse.AttrOut, syscall.Errno) {
	var in fuse.GetAttrIn
	if fh != nil {
		in.Flags_ = fuse.FUSE_GETATTR_FH
		in.Fh_ = *fh
	}
	var out fuse.AttrOut
	errno := c.do(opGetattr, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Setattr sends SETATTR. The header in `in` is overwritten.
func (c *Client) Setattr(caller *fuse.Caller, node uint64, in *fuse.SetAttrIn) (fuse.AttrOut, syscall.Errno) {
	var out fuse.AttrOut
	errno := c.do(opSetattr, node, caller, unsafe.Pointer(in), unsafe.Sizeof(*in), nil,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Readlink sends READLINK.
func (c *Client) Readlink(caller *fuse.Caller, node uint64) (string, syscall.Errno) {
	var in fuse.InHeader
	buf := make([]byte, 4096)
	n, errno := c.doN(opReadlink, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, buf)
	if errno != 0 {
		return "", errno
	}
	return string(buf[:n]), errno
}

// Symlink sends SYMLINK.
func (c *Client) Symlink(caller *fuse.Caller, parent uint64, name, target string) (fuse.EntryOut, syscall.Errno) {
	var in fuse.InHeader
	var out fuse.EntryOut
	payload, errno := cstr(name, target)
	if errno != 0 {
		return out, errno
	}
	errno = c.do(opSymlink, parent, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), payload,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Mkdir sends MKDIR.
func (c *Client) Mkdir(caller *fuse.Caller, parent uint64, name string, mode uint32) (fuse.EntryOut, syscall.Errno) {
	in := fuse.MkdirIn{Mode: mode}
	var out fuse.EntryOut
	payload, errno := cstr(name)
	if errno != 0 {
		return out, errno
	}
	errno = c.do(opMkdir, parent, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), payload,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Unlink sends UNLINK.
func (c *Client) Unlink(caller *fuse.Caller, parent uint64, name string) syscall.Errno {
	var in fuse.InHeader
	payload, errno := cstr(name)
	if errno != 0 {
		return errno
	}
	return c.do(opUnlink, parent, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), payload, nil, 0, nil)
}

// Rmdir sends RMDIR.
func (c *Client) Rmdir(caller *fuse.Caller, parent uint64, name string) syscall.Errno {
	var in fuse.InHeader
	payload, errno := cstr(name)
	if errno != 0 {
		return errno
	}
	return c.do(opRmdir, parent, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), payload, nil, 0, nil)
}

// Rename sends RENAME, which replaces an existing destination.
func (c *Client) Rename(caller *fuse.Caller, parent uint64, name string, newParent uint64, newName string) syscall.Errno {
	in := fuse.RenameIn{Newdir: newParent}
	// RENAME (as opposed to RENAME2) has no flags field.
	size := unsafe.Offsetof(in.Flags)
	payload, errno := cstr(name, newName)
	if errno != 0 {
		return errno
	}
	return c.do(opRename, parent, caller, unsafe.Pointer(&in), size, payload, nil, 0, nil)
}

// Open sends OPEN.
func (c *Client) Open(caller *fuse.Caller, node uint64, flags uint32) (fuse.OpenOut, syscall.Errno) {
	in := fuse.OpenIn{Flags: flags}
	var out fuse.OpenOut
	errno := c.do(opOpen, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Opendir sends OPENDIR.
func (c *Client) Opendir(caller *fuse.Caller, node uint64) (fuse.OpenOut, syscall.Errno) {
	in := fuse.OpenIn{}
	var out fuse.OpenOut
	errno := c.do(opOpendir, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Create sends CREATE.
func (c *Client) Create(caller *fuse.Caller, parent uint64, name string, flags, mode uint32) (fuse.EntryOut, fuse.OpenOut, syscall.Errno) {
	in := fuse.CreateIn{Flags: flags, Mode: mode}
	var out struct {
		fuse.EntryOut
		fuse.OpenOut
	}
	payload, errno := cstr(name)
	if errno != 0 {
		return out.EntryOut, out.OpenOut, errno
	}
	errno = c.do(opCreate, parent, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), payload,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out.EntryOut, out.OpenOut, errno
}

// Read sends READ requests to fill dest, stopping at a short read.
func (c *Client) Read(caller *fuse.Caller, node, fh uint64, off uint64, dest []byte) (int, syscall.Errno) {
	total := 0
	for total < len(dest) {
		chunk := dest[total:min(len(dest), total+c.MaxWrite())]
		in := fuse.ReadIn{Fh: fh, Offset: off + uint64(total), Size: uint32(len(chunk))}
		n, errno := c.doN(opRead, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, chunk)
		if errno != 0 {
			if total > 0 {
				break
			}
			return 0, errno
		}
		total += n
		if n < len(chunk) {
			break
		}
	}
	return total, 0
}

// Write sends WRITE requests for data.
func (c *Client) Write(caller *fuse.Caller, node, fh uint64, off uint64, data []byte) (int, syscall.Errno) {
	total := 0
	for total < len(data) {
		chunk := data[total:min(len(data), total+c.MaxWrite())]
		in := fuse.WriteIn{Fh: fh, Offset: off + uint64(total), Size: uint32(len(chunk))}
		var out fuse.WriteOut
		errno := c.do(opWrite, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), chunk,
			unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
		if errno != 0 {
			if total > 0 {
				break
			}
			return 0, errno
		}
		total += int(out.Size)
		if int(out.Size) < len(chunk) {
			break
		}
	}
	return total, 0
}

// Statfs sends STATFS.
func (c *Client) Statfs(caller *fuse.Caller, node uint64) (fuse.StatfsOut, syscall.Errno) {
	var in fuse.InHeader
	var out fuse.StatfsOut
	errno := c.do(opStatfs, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil,
		unsafe.Pointer(&out), unsafe.Sizeof(out), nil)
	return out, errno
}

// Flush sends FLUSH.
func (c *Client) Flush(caller *fuse.Caller, node, fh uint64) syscall.Errno {
	in := fuse.FlushIn{Fh: fh}
	return c.do(opFlush, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, nil)
}

// Fsync sends FSYNC.
func (c *Client) Fsync(caller *fuse.Caller, node, fh uint64) syscall.Errno {
	in := fuse.FsyncIn{Fh: fh}
	return c.do(opFsync, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, nil)
}

// Release sends RELEASE.
func (c *Client) Release(node, fh uint64, flags uint32) {
	in := fuse.ReleaseIn{Fh: fh, Flags: flags}
	c.do(opRelease, node, nil, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, nil)
}

// Releasedir sends RELEASEDIR.
func (c *Client) Releasedir(node, fh uint64) {
	in := fuse.ReleaseIn{Fh: fh}
	c.do(opReleasedir, node, nil, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, nil)
}

// DirEntry is an entry from READDIR or READDIRPLUS. Entry is only
// set for READDIRPLUS; if Entry.NodeId is non-zero, the entry
// counts as a lookup, except for "." and "..".
type DirEntry struct {
	fuse.DirEntry
	Entry fuse.EntryOut
}

// Readdir sends one READDIR or READDIRPLUS request, starting at
// off. An empty result marks the end of the directory.
func (c *Client) Readdir(caller *fuse.Caller, node, fh uint64, off uint64, plus bool) ([]DirEntry, syscall.Errno) {
	op := uint32(opReaddir)
	if plus {
		op = opReaddirplus
	}
	in := fuse.ReadIn{Fh: fh, Offset: off, Size: 64 * 1024}
	buf := make([]byte, in.Size)
	n, errno := c.doN(op, node, caller, unsafe.Pointer(&in), unsafe.Sizeof(in), nil, nil, 0, buf)
	if errno != 0 {
		return nil, errno
	}
	buf = buf[:n]

	var ents []fuse.DirEntry
	var outs []fuse.EntryOut
	var err error
	if plus {
		ents, outs, err = fuse.ParseDirLookupEntries(buf)
	} else {
		ents, err = fuse.ParseDirEntries(buf)
	}
	if err != nil {
		return nil, syscall.EIO
	}
	res := make([]DirEntry, len(ents))
	for i, e := range ents {
		res[i].DirEntry = e
		if plus {
			res[i].Entry = outs[i]
		}
	}
	return res, 0
}
