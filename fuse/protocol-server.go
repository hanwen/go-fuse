// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"sync"
)

// protocolServer bridges from the FUSE datatypes to a RawFileSystem
type protocolServer struct {
	fileSystem RawFileSystem

	interruptMu    sync.Mutex
	reqInflight    []*request
	connectionDead bool

	latencies LatencyMap

	kernelSettings InitIn

	opts *MountOptions

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

	if req.inHeader().NodeId == pollHackInode ||
		req.inHeader().NodeId == FUSE_ROOT_ID && h.FileNames > 0 && req.filename() == pollHackName {
		doPollHackLookup(ms, req)
	} else if req.status.Ok() && h.Func == nil {
		ms.opts.Logger.Printf("Unimplemented opcode %v", operationName(req.inHeader().Opcode))
		req.status = ENOSYS
	} else if req.status.Ok() {
		h.Func(ms, req)
	}

	// Forget/NotifyReply do not wait for reply from filesystem server.
	switch req.inHeader().Opcode {
	case _OP_FORGET, _OP_BATCH_FORGET, _OP_NOTIFY_REPLY:
		req.suppressReply = true
	case _OP_INTERRUPT:
		// ? what other status can interrupt generate?
		if req.status.Ok() {
			req.suppressReply = true
		}
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
	if req.inHeader().Opcode == _OP_INIT && ms.kernelSettings.Minor <= 22 {
		// v8-v22 don't have TimeGran and further fields.
		// This includes osxfuse (a.k.a. macfuse).
		req.outHeader().Length = uint32(sizeOfOutHeader) + 24
	}
	if req.fdData != nil && ms.opts.DisableSplice {
		req.outPayload, req.status = req.fdData.Bytes(req.outPayload)
		req.fdData = nil
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
	return &ProtocolServer{
		protocolServer: protocolServer{
			fileSystem:  fs,
			retrieveTab: make(map[uint64]*retrieveCacheRequest),
			opts:        opts,
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

// HandleRequest parses the iov in `in`, calls into the raw
// filesystem, and puts the result in `out` which should have enough
// space.
//
// EXPERIMENTAL: not subject to API stability.
func (ps *ProtocolServer) HandleRequest(in [][]byte,
	out [][]byte) (int, Status) {
	inTogether := make([]byte, iovLen(in))
	copied := 0
	for _, e := range in {
		n := copy(inTogether[copied:], e)
		copied += n
	}
	h, inSize, outSize, outPayloadSize, errno := parseRequest(inTogether, nil)
	if errno != 0 {
		return 0, errno
	}
	req := request{
		cancel:     make(chan struct{}),
		inputBuf:   inTogether[:inSize],
		outputBuf:  make([]byte, outSize+int(sizeOfOutHeader)),
		outPayload: make([]byte, outPayloadSize), // todo: use IOV passed-in here.
		inPayload:  inTogether[inSize:],
	}
	ps.protocolServer.handleRequest(h, &req)
	return iovCopy(out, [][]byte{req.outputBuf, req.outPayload}), 0
}

func iovCopy(dest [][]byte, src [][]byte) int {
	var s, d []byte
	var copied int
	for {
		if len(src) == 0 || len(dest) == 0 {
			break
		}
		if len(s) == 0 {
			s = src[0]
			src = src[1:]
		}
		if len(d) == 0 {
			d = dest[0]
			dest = dest[1:]
		}
		n := copy(d, s)
		d = d[n:]
		s = s[n:]
		copied += n
	}
	return copied
}
