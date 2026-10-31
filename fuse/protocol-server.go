// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"sync"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/internal/iov"
)

// conn is the state of the connection with the kernel, as negotiated
// by the INIT request (struct fuse_conn in the kernel).
type conn struct {
	// in is the INIT request from the kernel.
	in InitIn

	// out is the reply to INIT.
	out InitOut

	notifier *notifier
}

// protocolServer bridges from the FUSE datatypes to a RawFileSystem
type protocolServer struct {
	fileSystem RawFileSystem

	conn *conn

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
}

func (ms *protocolServer) handleRequest(h *operationHandler, req *request) {
	req.ctx = newRequestContext(req)
	ms.addInflight(req)
	defer func() {
		ms.dropInflight(req)
		req.finish()
	}()

	if req.status.Ok() && ms.opts.Debug {
		ms.opts.Logger.Println(req.InputDebug())
	}

	if h == nil || h.Func == nil {
		c := req.inHeader().Opcode
		if c != _OP_COPY_FILE_RANGE_64 { // _OP_COPY_FILE_RANGE_64 is intentionally not supported.
			ms.opts.Logger.Printf("Unimplemented opcode %v", operationName(c))
		}
		req.status = ENOSYS
	} else if !req.status.Ok() {
		// Reply with the error from parsing the request.
	} else if req.inHeader().NodeId == pollHackInode ||
		req.inHeader().NodeId == FUSE_ROOT_ID && h.FileNames > 0 && req.filename() == pollHackName {
		doPollHackLookup(ms, req)
	} else {
		func() {
			defer func() {
				if r := recover(); r != nil {
					req.status = ms.opts.PanicHandler(r)
					if req.status == 0 {
						req.status = EIO
					}
				}
			}()
			h.Func(ms, req)
		}()
	}

	// Forget/NotifyReply do not wait for reply from filesystem server.
	switch req.inHeader().Opcode {
	case _OP_INTERRUPT:
		// ? what other status can interrupt generate?
		if req.status.Ok() {
			req.suppressReply = true
		}
	default:
		req.suppressReply = h != nil && h.SuppressReply
	}
	if req.status == EINTR {
		ms.interruptMu.Lock()
		dead := ms.connectionDead
		ms.interruptMu.Unlock()
		if dead {
			req.suppressReply = true
		}
	}
	if req.suppressReply {
		return
	}
	if req.readResult != nil && ms.opts.DisableSplice {
		_, vectored := req.readResult.(withSlice)
		_, readv := req.readResult.(withReadv)
		if !vectored && !(readv && ms.useReadv) {
			req.outPayload, req.status = req.readResult.Bytes(req.outPayload)
			req.readResult.Done()
			req.readResult = nil
		}
	}
	if req.status > OK && req.readResult != nil {
		req.readResult.Done()
		req.readResult = nil
	}

	req.serializeHeader(req.outPayloadSize())

	if ms.opts.Debug {
		ms.opts.Logger.Println(req.OutputDebug())
	}
}

func (ms *protocolServer) addInflight(req *request) {
	ms.interruptMu.Lock()
	defer ms.interruptMu.Unlock()
	req.inflightIndex = len(ms.reqInflight)
	ms.reqInflight = append(ms.reqInflight, req)
}

func (ms *protocolServer) dropInflight(req *request) {
	ms.interruptMu.Lock()
	defer ms.interruptMu.Unlock()
	this := req.inflightIndex
	last := len(ms.reqInflight) - 1
	if last != this {
		ms.reqInflight[this] = ms.reqInflight[last]
		ms.reqInflight[this].inflightIndex = this
	}
	ms.reqInflight = ms.reqInflight[:last]
}

func (ms *protocolServer) interruptRequest(unique uint64) Status {
	ms.interruptMu.Lock()
	defer ms.interruptMu.Unlock()

	// This is slow, but this operation is rare.
	for _, inflight := range ms.reqInflight {
		if unique == inflight.inHeader().Unique && !inflight.ctx.canceled.Load() {
			inflight.ctx.interrupt()
			return OK
		}
	}

	return EAGAIN
}

func (ms *protocolServer) cancelAll() {
	ms.interruptMu.Lock()
	defer ms.interruptMu.Unlock()
	ms.connectionDead = true
	for _, req := range ms.reqInflight {
		req.ctx.interrupt()
	}
	// Leave ms.reqInflight alone, or dropInflight will barf.
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
// EXPERIMENTAL: not subject to API stability.
func NewProtocolServer(fs RawFileSystem, opts *MountOptions) *ProtocolServer {
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
			conn:       &conn{notifier: newNotifier(nil, &optsCopy)},
		},
	}
}

// HandleRequest parses the iov in `in`, calls into the raw
// filesystem, and puts the result in `out`. The iovs are treated
// as byte streams. The return value is the number of bytes written.
//
// EXPERIMENTAL: not subject to API stability.
func (ps *ProtocolServer) HandleRequest(in [][]byte, out [][]byte) (int, Status) {
	req := &requestAlloc{}

	total := iov.Len(in)
	n := iov.CopyFrom(req.smallInputBuf[:min(total, len(req.smallInputBuf))], in, 0)
	if n < int(unsafe.Sizeof(InHeader{})) {
		ps.opts.Logger.Printf("request too short: %v", iov.Lens(in))
		return 0, EIO
	}
	h, inSize, outSize, outPayloadSize, code := parseRequest(req.smallInputBuf[:n], ps.conn.out.Flags64())
	if !code.Ok() && code != ENOSYS {
		ps.opts.Logger.Printf("parseRequest: %v", code)
	}
	req.inputBuf = req.smallInputBuf[:n]
	req.status = code
	if code.Ok() {
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
		if !req.status.Ok() {
			ps.opts.Logger.Printf("op %s: bad request extension: %v", h.Name, req.status)
		}
	}

	req.outHeaderBuf = req.outHeaderInline[:]
	req.outDataBuf = req.outDataInline[:outSize]
	clear(req.outHeaderBuf)
	clear(req.outDataBuf)

	var direct []byte
	suppressReply := h != nil && h.SuppressReply
	if !suppressReply && req.status.Ok() {
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
		if !code.Ok() {
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

func readIntoIov(rr ReadResult, dst [][]byte) (int, Status) {
	switch r := rr.(type) {
	case withReadv:
		return r.Readv(dst)
	case withSlice:
		slices, code := r.Slices()
		if !code.Ok() {
			return 0, code
		}
		return iov.CopyTo(dst, slices...), OK
	}
	return 0, EIO
}
