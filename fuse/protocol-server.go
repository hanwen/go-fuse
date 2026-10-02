// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"sync"
	"syscall"
	"unsafe"
)

// protocolServer bridges from the FUSE datatypes to a RawFileSystem
type protocolServer struct {
	fileSystem RawFileSystem

	writev func([][]byte) (int, syscall.Errno)

	interruptMu    sync.Mutex
	reqInflight    []*request
	connectionDead bool

	kernelSettings InitIn

	// If set, handleRequest does not flatten withReadv results
	// with Bytes(), but leaves them in req.readResult, so
	// ProtocolServer.HandleRequest can Readv them straight into
	// the reply iov.
	useReadv bool

	// Capabilities sent in InitOut.
	negotiatedFlags uint64

	opts *MountOptions

	// Pools for []byte
	buffers bufferPool

	writevCopyOnce sync.Once

	// in-flight notify-retrieve queries
	retrieveMu   sync.Mutex
	retrieveNext uint64
	retrieveTab  map[uint64]*retrieveCacheRequest // notifyUnique -> retrieve request
}

func (ms *protocolServer) handleRequest(h *operationHandler, req *request) {
	ms.addInflight(req)
	defer ms.dropInflight(req)

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
			req.ctx = Context{Caller: req.inHeader().Caller, Cancel: req.cancel}
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
		if unique == inflight.inHeader().Unique && !inflight.interrupted {
			close(inflight.cancel)
			inflight.interrupted = true
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
		if !req.interrupted {
			close(req.cancel)
			req.interrupted = true
		}
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
			fileSystem:  fs,
			retrieveTab: make(map[uint64]*retrieveCacheRequest),
			opts:        &optsCopy,
			useReadv:    true,
		},
	}
}

func iovLen(iov [][]byte) int {
	var r int
	for _, e := range iov {
		r += len(e)
	}
	return r
}

// iovSlice returns iov[off:off+n] if it lies within one element.
func iovSlice(iov [][]byte, off, n int) ([]byte, bool) {
	for _, e := range iov {
		if off < len(e) {
			if off+n <= len(e) {
				return e[off : off+n], true
			}
			return nil, false
		}
		off -= len(e)
	}
	return nil, n == 0
}

func copyFromIov(dst []byte, iov [][]byte, off int) int {
	var n int
	for _, e := range iov {
		if len(dst) == n {
			break
		}
		if off >= len(e) {
			off -= len(e)
			continue
		}
		n += copy(dst[n:], e[off:])
		off = 0
	}
	return n
}

type iovWriter struct {
	dst     [][]byte
	i, off  int
	written int
}

func (w *iovWriter) next(n int) []byte {
	for w.i < len(w.dst) && w.off == len(w.dst[w.i]) {
		w.i++
		w.off = 0
	}
	if w.i == len(w.dst) {
		return nil
	}
	chunk := w.dst[w.i][w.off:]
	chunk = chunk[:min(len(chunk), n)]
	w.off += len(chunk)
	w.written += len(chunk)
	return chunk
}

func (w *iovWriter) write(src []byte) int {
	var n int
	for n < len(src) {
		chunk := w.next(len(src) - n)
		if chunk == nil {
			break
		}
		n += copy(chunk, src[n:])
	}
	return n
}

func (w *iovWriter) skip(n int) {
	for n > 0 {
		chunk := w.next(n)
		if chunk == nil {
			break
		}
		n -= len(chunk)
	}
}

// sliceIov returns the iov for bytes [off, off+n).
func sliceIov(iov [][]byte, off, n int) [][]byte {
	var r [][]byte
	for _, e := range iov {
		if n == 0 {
			break
		}
		if off >= len(e) {
			off -= len(e)
			continue
		}
		e = e[off:]
		off = 0
		e = e[:min(len(e), n)]
		n -= len(e)
		r = append(r, e)
	}
	return r
}

func copyToIov(dst [][]byte, src ...[]byte) int {
	w := iovWriter{dst: dst}
	for _, s := range src {
		w.write(s)
	}
	return w.written
}

// HandleRequest parses the iov in `in`, calls into the raw
// filesystem, and puts the result in `out`. The iovs are treated
// as byte streams. The return value is the number of bytes written.
//
// EXPERIMENTAL: not subject to API stability.
func (ps *ProtocolServer) HandleRequest(in [][]byte, out [][]byte) (int, Status) {
	req := &requestAlloc{
		request: request{
			cancel: make(chan struct{}),
		},
	}

	total := iovLen(in)
	n := copyFromIov(req.smallInputBuf[:min(total, len(req.smallInputBuf))], in, 0)
	if n < int(unsafe.Sizeof(InHeader{})) {
		ps.opts.Logger.Printf("request too short: %v", iovLens(in))
		return 0, EIO
	}
	h, inSize, outSize, outPayloadSize, code := parseRequest(req.smallInputBuf[:n], &ps.kernelSettings, ps.negotiatedFlags)
	if !code.Ok() && code != ENOSYS {
		ps.opts.Logger.Printf("parseRequest: %v", code)
	}
	req.inputBuf = req.smallInputBuf[:n]
	req.status = code
	if code.Ok() {
		req.inputBuf = req.inputBuf[:inSize]
		payloadLen := total - inSize
		if b, ok := iovSlice(in, inSize, payloadLen); ok {
			req.inPayload = b
		} else if req.inHeader().Opcode == _OP_WRITE {
			req.inPayloadIov = sliceIov(in, inSize, payloadLen)
		} else {
			// IOCTL data comes in one element per page. Other
			// payloads (names, xattr values, BATCH_FORGET
			// entries, extensions) are only split if they
			// cross a guest memory region boundary.
			req.inPayload = make([]byte, payloadLen)
			copyFromIov(req.inPayload, in, inSize)
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
		if iovLen(out) < need {
			ps.opts.Logger.Printf("op %v: got out iov %v, need %d bytes", h.Name, iovLens(out), need)
			return 0, EIO
		}
		if outPayloadSize > 0 {
			if b, ok := iovSlice(out, int(sizeOfOutHeader)+outSize, outPayloadSize); ok {
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
	w := iovWriter{dst: out}
	if req.readResult != nil {
		// The size is only known after the read, so redo the header.
		payloadOff := int(sizeOfOutHeader) + len(req.outDataBuf)
		dst := sliceIov(out, payloadOff, req.readResult.Size())
		n, code := readIntoIov(req.readResult, dst)
		req.readResult.Done()
		req.readResult = nil
		if !code.Ok() {
			req.status = code
			n = 0
		}
		req.serializeHeader(n)
		w.write(req.outHeaderBuf)
		w.write(req.outDataBuf)
		w.skip(n)
		return w.written, OK
	}

	w.write(req.outHeaderBuf)
	w.write(req.outDataBuf)
	if len(req.outPayload) > 0 && len(direct) > 0 && &req.outPayload[0] == &direct[0] {
		// The filesystem filled the out iov in place; count
		// the bytes without copying.
		w.skip(len(req.outPayload))
	} else {
		w.write(req.outPayload)
	}
	return w.written, OK
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
		return copyToIov(dst, slices...), OK
	}
	return 0, EIO
}

func iovLens(in [][]byte) []int {
	var lens []int
	for _, b := range in {
		lens = append(lens, len(b))
	}
	return lens
}
