// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"log"
	"sync"
	"syscall"
)

// protocolServer bridges from the FUSE datatypes to a RawFileSystem
type protocolServer struct {
	fileSystem RawFileSystem

	writev func([][]byte) (int, syscall.Errno)

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
// space. The return value is the number of response bytes written.
//
// EXPERIMENTAL: not subject to API stability.
func (ps *ProtocolServer) HandleRequest(in [][]byte,
	out [][]byte) (int, Status) {

	// for virtiofs, we get
	//
	// 2026/04/17 13:34:40 in: 40 32
	// 2026/04/17 13:34:40 out: 16 16 4096
	//
	// ie. the iov looks like {header , variable size, payload},
	// for both input and output.
	//
	// Our input data types have the InHeader embedded in the FooIn
	// types, so we can never make this efficient.
	//
	// The output types don't have the output header embedded, so we could do something here.

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
		cancel:        make(chan struct{}),
		inputBuf:      inTogether[:inSize],
		inPayload:     inTogether[inSize:],
		suppressReply: h.SuppressReply,
	}

	startOut := out
	if !h.SuppressReply {
		if len(out) > 0 && len(out[0]) == int(sizeOfOutHeader) {
			req.outHeaderBuf = out[0]
			out = out[1:]
		} else {
			log.Panicf("op %v: got %v, out iov should start with 16 bytes", h.Name, iovLens(startOut))
		}

		if outSize > 0 {
			if len(out) > 0 && len(out[0]) == outSize {
				req.outDataBuf = out[0]
				out = out[1:]
			} else {
				log.Panicf("op %v: got %v, outData iov should have %d bytes", h.Name, iovLens(startOut), outSize)
			}
		}

		if len(out) > 0 {
			if len(out[0]) < outPayloadSize {
				log.Panicf("op %s: got %v, payload iov should have %d bytes", h.Name, iovLens(startOut), outPayloadSize)
			}
			req.outPayload = out[0]
			out = out[1:]
		} else if outPayloadSize != 0 {
			log.Panicf("got %v, payload iov should have %d bytes", iovLens(startOut), outPayloadSize)
		}
	}
	ps.protocolServer.handleRequest(h, &req)
	return iovLen(startOut), 0
}

func iovLens(in [][]byte) []int {
	var lens []int
	for _, b := range in {
		lens = append(lens, len(b))
	}
	return lens
}

func iovCopy(dest [][]byte, src [][]byte) int {
	var s, d []byte
	var copied int
	for {
		if len(s)+len(src) == 0 || len(d)+len(dest) == 0 {
			break
		}
		if len(s) == 0 {
			s = src[0]
			src = src[1:]
			continue
		}
		if len(d) == 0 {
			d = dest[0]
			dest = dest[1:]
			continue
		}
		n := copy(d, s)
		d = d[n:]
		s = s[n:]
		copied += n
	}
	return copied
}
