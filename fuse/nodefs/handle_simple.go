// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sync"
)

// simpleHandleMap is a simple, low-performance handle map that never reuses
// handle numbers.
type simpleHandleMap struct {
	sync.Mutex
	entries    map[uint64]*handled
	nextHandle uint64
}

func newSimpleHandleMap() handleMap {
	return &simpleHandleMap{
		entries: make(map[uint64]*handled),
		// Avoid handing out ID 0 and 1, like portableHandleMap does.
		nextHandle: 2,
	}
}

func (s *simpleHandleMap) Register(obj *handled) (handle, generation uint64) {
	s.Lock()
	defer s.Unlock()
	// The object already has a handle
	if obj.count != 0 {
		if obj.handle == 0 {
			log.Panicf("bug: count=%d handle=%d", obj.count, obj.handle)
		}
		obj.count++
		return obj.handle, 0
	}
	// Create a new handle
	obj.count = 1
	obj.handle = s.nextHandle
	s.entries[s.nextHandle] = obj
	s.nextHandle++
	return obj.handle, 0
}

// Count returns the number of currently used handles
func (s *simpleHandleMap) Count() int {
	s.Lock()
	defer s.Unlock()
	return len(s.entries)
}

// Handle gets the object's uint64 handle.
func (s *simpleHandleMap) Handle(obj *handled) (handle uint64) {
	s.Lock()
	defer s.Unlock()
	if obj.count == 0 {
		return 0
	}
	return obj.handle
}

// Decode retrieves a stored object from its uint64 handle.
func (s *simpleHandleMap) Decode(handle uint64) *handled {
	s.Lock()
	defer s.Unlock()
	return s.entries[handle]
}

// Forget decrements the reference counter for "handle" by "count" and drops
// the object if the refcount reaches zero.
// Returns a boolean whether the object was dropped and the object itself.
func (s *simpleHandleMap) Forget(handle uint64, count int) (forgotten bool, obj *handled) {
	s.Lock()
	defer s.Unlock()
	obj = s.entries[handle]
	obj.count -= count
	if obj.count < 0 {
		log.Panicf("underflow: handle %d, count %d,  obj.count %d", handle, count, obj.count)
	}
	if obj.count > 0 {
		return false, obj
	}
	// count is zero, drop the reference
	delete(s.entries, handle)
	obj.handle = 0
	return true, obj
}

// Has checks if the uint64 handle is stored.
func (s *simpleHandleMap) Has(handle uint64) bool {
	s.Lock()
	defer s.Unlock()
	_, ok := s.entries[handle]
	return ok
}
