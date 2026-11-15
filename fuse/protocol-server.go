// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v3/internal/iov"
)

// protocolServer bridges from the FUSE datatypes to a RawFileSystem
type protocolServer struct {
	fileSystem RawFileSystem

	// Before INIT, notify and backing are as supplied by the
	// transport; afterwards, they are gated by the kernel version
	// and possibly wrapped by RawFileSystem.OnConnect.
	conn *Conn

	interruptMu    sync.Mutex
	reqInflight    []*request
	connectionDead bool

	// If set, handleRequest does not flatten withReadv results
	// with Bytes(), but leaves them in req.readResult, so
	// ProtocolServer.HandleRequest can Readv them straight into
	// the reply iov.
	useReadv bool

	opts *MountOptions

	// Pools for []byte
	buffers bufferPool

	writevCopyOnce sync.Once

	unmountOnce sync.Once
}

// onUnmount calls RawFileSystem.OnUnmount, once.
func (ps *protocolServer) onUnmount() {
	ps.unmountOnce.Do(ps.fileSystem.OnUnmount)
}

func (ps *protocolServer) handleRequest(h *operationHandler, req *request) {
	req.ctx = newRequestContext(req)
	ps.addInflight(req)
	defer func() {
		ps.dropInflight(req)
		req.finish()
	}()

	if req.status == OK && ps.opts.Debug {
		ps.opts.Logger.Println(req.InputDebug())
	}

	if h == nil || h.Func == nil {
		c := req.inHeader().Opcode
		if c != _OP_COPY_FILE_RANGE_64 { // _OP_COPY_FILE_RANGE_64 is intentionally not supported.
			ps.opts.Logger.Printf("Unimplemented opcode %v", operationName(c))
		}
		req.status = ENOSYS
	} else if req.status != OK {
		// Reply with the error from parsing the request.
	} else if req.inHeader().NodeId == pollHackInode ||
		req.inHeader().NodeId == FUSE_ROOT_ID && h.FileNames > 0 && req.filename() == pollHackName {
		doPollHackLookup(ps, req)
	} else {
		func() {
			defer func() {
				if r := recover(); r != nil {
					req.status = ps.opts.PanicHandler(r)
					if req.status == 0 {
						req.status = EIO
					}
				}
			}()
			h.Func(ps, req)
		}()
	}

	// Forget/NotifyReply do not wait for reply from filesystem server.
	switch req.inHeader().Opcode {
	case _OP_INTERRUPT:
		// ? what other status can interrupt generate?
		if req.status == OK {
			req.suppressReply = true
		}
	default:
		req.suppressReply = h != nil && h.SuppressReply
	}
	if req.status == EINTR {
		ps.interruptMu.Lock()
		dead := ps.connectionDead
		ps.interruptMu.Unlock()
		if dead {
			req.suppressReply = true
		}
	}
	if req.suppressReply {
		return
	}
	if req.readResult != nil && ps.opts.DisableSplice {
		_, vectored := req.readResult.(withSlice)
		_, readv := req.readResult.(withReadv)
		if !vectored && !(readv && ps.useReadv) {
			req.outPayload, req.status = req.readResult.Bytes(req.outPayload)
			req.readResult.Done()
			req.readResult = nil
		}
	}
	if req.status != OK && req.readResult != nil {
		req.readResult.Done()
		req.readResult = nil
	}

	req.serializeHeader(req.outPayloadSize())

	if ps.opts.Debug {
		ps.opts.Logger.Println(req.OutputDebug())
	}
}

func (ps *protocolServer) addInflight(req *request) {
	ps.interruptMu.Lock()
	defer ps.interruptMu.Unlock()
	req.inflightIndex = len(ps.reqInflight)
	ps.reqInflight = append(ps.reqInflight, req)
}

func (ps *protocolServer) dropInflight(req *request) {
	ps.interruptMu.Lock()
	defer ps.interruptMu.Unlock()
	this := req.inflightIndex
	last := len(ps.reqInflight) - 1
	if last != this {
		ps.reqInflight[this] = ps.reqInflight[last]
		ps.reqInflight[this].inflightIndex = this
	}
	ps.reqInflight = ps.reqInflight[:last]
}

func (ps *protocolServer) interruptRequest(unique uint64) syscall.Errno {
	ps.interruptMu.Lock()
	defer ps.interruptMu.Unlock()

	// This is slow, but this operation is rare.
	for _, inflight := range ps.reqInflight {
		if unique == inflight.inHeader().Unique && !inflight.ctx.canceled.Load() {
			inflight.ctx.interrupt()
			return OK
		}
	}

	return EAGAIN
}

func (ps *protocolServer) cancelAll() {
	ps.interruptMu.Lock()
	defer ps.interruptMu.Unlock()
	ps.connectionDead = true
	for _, req := range ps.reqInflight {
		req.ctx.interrupt()
	}
	// Leave ps.reqInflight alone, or dropInflight will barf.
}

// ProtocolServer bridges from FUSE request/response types to the
// Go-FUSE RawFileSystem API calls.
//
// EXPERIMENTAL: not subject to API stability.
type ProtocolServer struct {
	protocolServer
}

// NewProtocolServer creates a ProtocolServer for the RawFileSystem.
//
// Notify holds the notifications that the file system may send, for
// transports that handle them in-process. If it is nil, or for nil
// functions in it, notifications return ENOSYS.
//
// EXPERIMENTAL: not subject to API stability.
func NewProtocolServer(fs RawFileSystem, opts *MountOptions, notify *Notify) *ProtocolServer {
	// ProtocolServer has no pipe, so splicing READ results to the
	// caller is not possible; force the in-process READ path.
	optsCopy := *opts
	optsCopy.setDefaults(fs)
	optsCopy.DisableSplice = true

	return &ProtocolServer{
		protocolServer: protocolServer{
			fileSystem: fs,
			opts:       &optsCopy,
			useReadv:   true,
			conn: &Conn{
				notify:  inProcessNotify(notify),
				backing: enosysBacking(),
			},
		},
	}
}

// inProcessNotify returns a copy of n with nil functions replaced by
// ENOSYS stubs.
func inProcessNotify(n *Notify) *Notify {
	stub := enosysNotify()
	if n == nil {
		return stub
	}
	c := *n
	c.notifier = nil
	if c.InvalInode == nil {
		c.InvalInode = stub.InvalInode
	}
	if c.InvalEntry == nil {
		c.InvalEntry = stub.InvalEntry
	}
	if c.Delete == nil {
		c.Delete = stub.Delete
	}
	if c.Prune == nil {
		c.Prune = stub.Prune
	}
	if c.Store == nil {
		c.Store = stub.Store
	}
	if c.Retrieve == nil {
		c.Retrieve = stub.Retrieve
	}
	return &c
}

// HandleRequest parses the iov in `in`, calls into the raw
// filesystem, and puts the result in `out`. The iovs are treated
// as byte streams. The return value is the number of bytes written.
//
// EXPERIMENTAL: not subject to API stability.
func (ps *ProtocolServer) HandleRequest(in [][]byte, out [][]byte) (int, syscall.Errno) {
	req := &requestAlloc{}

	total := iov.Len(in)
	n := iov.CopyFrom(req.smallInputBuf[:min(total, len(req.smallInputBuf))], in, 0)
	if n < int(unsafe.Sizeof(InHeader{})) {
		ps.opts.Logger.Printf("request too short: %v", iov.Lens(in))
		return 0, EIO
	}
	h, inSize, outSize, outPayloadSize, code := parseRequest(req.smallInputBuf[:n], ps.conn.out.Flags64())
	if code != OK && code != ENOSYS {
		ps.opts.Logger.Printf("parseRequest: %v", code)
	}
	req.inputBuf = req.smallInputBuf[:n]
	req.status = code
	if code == OK {
		req.inputBuf = req.inputBuf[:inSize]
		payloadLen := total - inSize
		if b, ok := iov.Slice(in, inSize, payloadLen); ok {
			req.inPayload = b
		} else if req.inHeader().Opcode == _OP_WRITE {
			req.inPayloadIov = iov.Subslice(in, inSize, payloadLen)
		} else {
			// IOCTL data comes in one element per page. Other
			// payloads (names, xattr values, BATCH_FORGET
			// entries, extensions) are only split if they
			// cross a guest memory region boundary.
			req.inPayload = make([]byte, payloadLen)
			iov.CopyFrom(req.inPayload, in, inSize)
		}
		req.extendInput(int(h.InputSize))
		req.status = req.splitExt(&req.extInline)
		if req.status != OK {
			ps.opts.Logger.Printf("op %s: bad request extension: %v", h.Name, req.status)
		}
	}

	req.outHeaderBuf = req.outHeaderInline[:]
	req.outDataBuf = req.outDataInline[:outSize]
	clear(req.outHeaderBuf)
	clear(req.outDataBuf)

	var direct []byte
	suppressReply := h != nil && h.SuppressReply
	if !suppressReply && req.status == OK {
		need := int(sizeOfOutHeader) + outSize + outPayloadSize
		if iov.Len(out) < need {
			ps.opts.Logger.Printf("op %v: got out iov %v, need %d bytes", h.Name, iov.Lens(out), need)
			return 0, EIO
		}
		if outPayloadSize > 0 {
			if b, ok := iov.Slice(out, int(sizeOfOutHeader)+outSize, outPayloadSize); ok {
				direct = b
				req.outPayload = b
			} else {
				req.outPayload = ps.buffers.AllocBuffer(uint32(outPayloadSize))
				defer ps.buffers.FreeBuffer(req.outPayload)
			}
		}
	}

	ps.protocolServer.handleRequest(h, &req.request)
	if req.suppressReply {
		return 0, OK
	}

	// Per the virtio spec, vring_used_elem.len should hold the
	// number of bytes the device wrote to the descriptor
	// chain. Returning more  inflates the live-migration dirty-page
	// log and violates the spec.
	w := iov.Writer{Dst: out}
	if req.readResult != nil {
		// The size is only known after the read, so redo the header.
		payloadOff := int(sizeOfOutHeader) + len(req.outDataBuf)
		dst := iov.Subslice(out, payloadOff, req.readResult.Size())
		n, code := readIntoIov(req.readResult, dst)
		req.readResult.Done()
		req.readResult = nil
		if code != OK {
			req.status = code
			n = 0
		}
		req.serializeHeader(n)
		w.Write(req.outHeaderBuf)
		w.Write(req.outDataBuf)
		w.Skip(n)
		return w.Written, OK
	}

	w.Write(req.outHeaderBuf)
	w.Write(req.outDataBuf)
	if len(req.outPayload) > 0 && len(direct) > 0 && &req.outPayload[0] == &direct[0] {
		// The filesystem filled the out iov in place; count
		// the bytes without copying.
		w.Skip(len(req.outPayload))
	} else {
		w.Write(req.outPayload)
	}
	return w.Written, OK
}

func readIntoIov(rr ReadResult, dst [][]byte) (int, syscall.Errno) {
	switch r := rr.(type) {
	case withReadv:
		return r.Readv(dst)
	case withSlice:
		slices, code := r.Slices()
		if code != OK {
			return 0, code
		}
		return iov.CopyTo(dst, slices...), OK
	}
	return 0, EIO
}
