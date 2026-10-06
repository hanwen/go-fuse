// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Call describes a FUSE request. Caller remains valid after the
// request finishes; the other accessors return false once the
// RawFileSystem method has returned.
type Call struct {
	// We copy this data: since it contains a UID/GID, it can be
	// considered a credential, and must be propagated in
	// contexts, even if the request has already been canceled.
	caller Caller

	// The rest may be larger, so don't copy the data; access it under lock instead
	req *request
	gen uint64
}

// requestContext is the context.Context passed to RawFileSystem
// methods. It is canceled on interrupt.
type requestContext struct {
	call Call

	// Interrupts are rare, so the cancelable context is created
	// lazily by Done().
	canceled atomic.Bool
	mu       sync.Mutex
	state    atomic.Pointer[cancelState]
}

type cancelState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func newRequestContext(req *request) *requestContext {
	return &requestContext{
		call: Call{
			caller: req.inHeader().Caller,
			req:    req,
			gen:    req.gen,
		},
	}
}

// CallFromContext returns the Call of the FUSE request.
func CallFromContext(ctx context.Context) (*Call, bool) {
	c, ok := ctx.Value(callKey).(*Call)
	return c, ok
}

func (c *Call) Caller() Caller {
	return c.caller
}

// SuppGroups returns the supplementary group sent with
// CAP_CREATE_SUPP_GROUP.
func (c *Call) SuppGroups() (groups []uint32, ok bool) {
	ok = c.withRequest(func(r *request) {
		if r.ext != nil {
			groups = append([]uint32(nil), r.ext.suppGroups...)
		}
	})
	return groups, ok
}

// SecCtx is a security context sent with CAP_SECURITY_CTX.
type SecCtx struct {
	Name  string
	Value []byte
}

func (c *Call) SecCtx() (ctxs []SecCtx, ok bool) {
	ok = c.withRequest(func(r *request) {
		if r.ext == nil {
			return
		}
		for _, s := range r.ext.secctx {
			ctxs = append(ctxs, SecCtx{
				Name:  string(s.name),
				Value: append([]byte(nil), s.value...),
			})
		}
	})
	return ctxs, ok
}

func (h *InHeader) inHeader() *InHeader {
	return h
}

// input is implemented by the XxxIn structs, which embed InHeader.
type input interface {
	inHeader() *InHeader
}

// Input copies the request's input struct into dst. It returns
// false if dst does not have the input type of the opcode.
func Input[T any, PT interface {
	*T
	input
}](c *Call, dst PT) bool {
	var ok bool
	return c.withRequest(func(r *request) {
		h := getHandler(r.inHeader().Opcode)
		if h == nil || h.InType == nil || reflect.TypeOf(h.InType) != reflect.TypeOf(dst).Elem() {
			return
		}
		// Inputs from older kernels may be shorter.
		copy(unsafe.Slice((*byte)(unsafe.Pointer(dst)), unsafe.Sizeof(*dst)), r.inputBuf)
		ok = true
	}) && ok
}

func (c *Call) withRequest(f func(r *request)) bool {
	c.req.mu.Lock()
	defer c.req.mu.Unlock()
	if c.req.gen != c.gen {
		return false
	}
	f(c.req)
	return true
}

func (c *requestContext) interrupt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.canceled.Store(true)
	if s := c.state.Load(); s != nil {
		s.cancel()
	}
}

func (c *requestContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *requestContext) Done() <-chan struct{} {
	if s := c.state.Load(); s != nil {
		return s.ctx.Done()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state.Load()
	if s == nil {
		ctx, cancel := context.WithCancel(context.Background())
		s = &cancelState{ctx, cancel}
		if c.canceled.Load() {
			cancel()
		}
		c.state.Store(s)
	}
	return s.ctx.Done()
}

func (c *requestContext) Err() error {
	if s := c.state.Load(); s != nil {
		return s.ctx.Err()
	}
	if c.canceled.Load() {
		return context.Canceled
	}
	return nil
}

// Deferring to the cancelCtx lets context.WithCancel attach derived
// contexts without starting a goroutine.
func (c *requestContext) Value(key any) any {
	switch key {
	case callKey:
		return &c.call
	case callerKey:
		return &c.call.caller
	}
	if s := c.state.Load(); s != nil {
		return s.ctx.Value(key)
	}
	return nil
}

type contextKey int

const (
	callKey contextKey = iota
	callerKey
)

// FromContext returns the Caller of the FUSE request.
func FromContext(ctx context.Context) (Caller, bool) {
	v, ok := ctx.Value(callerKey).(*Caller)
	if !ok {
		return Caller{}, false
	}
	return *v, true
}

// NewContext returns a context carrying caller.
func NewContext(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, callerKey, &caller)
}
