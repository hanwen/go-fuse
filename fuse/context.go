// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// requestContext is canceled on interrupt. Interrupts are rare, so
// the cancelable context is created lazily by Done().
type requestContext struct {
	caller   Caller
	canceled atomic.Bool

	mu    sync.Mutex
	state atomic.Pointer[cancelState]
}

type cancelState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func newRequestContext(req *request) *requestContext {
	return &requestContext{caller: req.inHeader().Caller}
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
	if key == callerKey {
		return &c.caller
	}
	if s := c.state.Load(); s != nil {
		return s.ctx.Value(key)
	}
	return nil
}

type callerKeyType struct{}

var callerKey callerKeyType

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
