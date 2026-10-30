// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"math"
	"sync"
	"syscall"
	"unsafe"
)

// notifier sends notifications through a writev function, and keeps
// track of in-flight NOTIFY_RETRIEVE requests.
type notifier struct {
	writev func([][]byte) (int, syscall.Errno)
	opts   *MountOptions

	retrieveMu   sync.Mutex
	retrieveNext uint64
	retrieveTab  map[uint64]*retrieveCacheRequest // notifyUnique -> retrieve request
}

func newNotifier(writev func([][]byte) (int, syscall.Errno), opts *MountOptions) *notifier {
	return &notifier{
		writev:      writev,
		opts:        opts,
		retrieveTab: make(map[uint64]*retrieveCacheRequest),
	}
}

// deliverReply hands a NOTIFY_REPLY from the kernel to the waiting
// Retrieve call.
func (ms *notifier) deliverReply(reply *NotifyRetrieveIn, payload []byte) {
	ms.retrieveMu.Lock()
	reading := ms.retrieveTab[reply.Unique]
	delete(ms.retrieveTab, reply.Unique)
	ms.retrieveMu.Unlock()

	badf := func(format string, argv ...any) {
		ms.opts.Logger.Printf("notify reply: "+format, argv...)
	}

	if reading == nil {
		badf("unexpected unique - ignoring")
		return
	}

	reading.n = 0
	reading.st = EIO
	defer close(reading.ready)

	if reading.nodeid != reply.NodeId {
		badf("inode mismatch: expected %s, got %s", reading.nodeid, reply.NodeId)
		return
	}

	if reading.offset != reply.Offset {
		badf("offset mismatch: expected @%d, got @%d", reading.offset, reply.Offset)
		return
	}

	if len(reading.dest) < len(payload) {
		badf("too much data: requested %db, got %db (will use only %db)", len(reading.dest), len(payload), len(reading.dest))
	}

	reading.n = copy(reading.dest, payload)
	reading.st = OK
}

// abort fails in-flight retrieves with ENODEV.
func (ms *notifier) abort() {
	// It is possible that umount comes in the middle - after retrieve
	// request was sent to kernel, but corresponding kernel reply has not
	// yet been read. We unblock all such readers and wake them up with ENODEV.
	ms.retrieveMu.Lock()
	rtab := ms.retrieveTab
	// retrieve attempts might be erroneously tried even after close
	// we have to keep retrieveTab !nil not to panic.
	ms.retrieveTab = make(map[uint64]*retrieveCacheRequest)
	ms.retrieveMu.Unlock()
	for _, reading := range rtab {
		reading.n = 0
		reading.st = ENODEV
		close(reading.ready)
	}
}

func notifyWrite(writev func([][]byte) (int, syscall.Errno), opts *MountOptions, req *request) Status {
	req.serializeHeader(req.outPayloadSize())

	if opts.Debug {
		opts.Logger.Println(req.OutputDebug())
	}

	_, errno := writev([][]byte{req.outHeaderBuf, req.outDataBuf, req.outPayload})

	if opts.Debug {
		h := getHandler(req.inHeader().Opcode)
		opts.Logger.Printf("Response %s: %v", h.Name, errno)
	}

	return Status(errno)
}

func (ms *notifier) notifyWrite(req *request) Status {
	if ms.writev == nil {
		return ENOSYS
	}
	return notifyWrite(ms.writev, ms.opts, req)
}

func newNotifyRequest(opcode uint32) *request {
	r := &request{
		inputBuf:     make([]byte, unsafe.Sizeof(InHeader{})),
		outHeaderBuf: make([]byte, sizeOfOutHeader),
		outDataBuf:   make([]byte, getHandler(opcode).OutputSize),
		status: map[uint32]Status{
			_OP_NOTIFY_INVAL_INODE:    NOTIFY_INVAL_INODE,
			_OP_NOTIFY_INVAL_ENTRY:    NOTIFY_INVAL_ENTRY,
			_OP_NOTIFY_STORE_CACHE:    NOTIFY_STORE_CACHE,
			_OP_NOTIFY_RETRIEVE_CACHE: NOTIFY_RETRIEVE_CACHE,
			_OP_NOTIFY_DELETE:         NOTIFY_DELETE,
			_OP_NOTIFY_PRUNE:          NOTIFY_PRUNE,
		}[opcode],
	}
	r.inHeader().Opcode = opcode
	return r
}

// InodeNotify invalidates the information associated with the inode
// (ie. data cache, attributes, etc.)
func (ms *notifier) InodeNotify(node uint64, off int64, length int64) Status {
	req := newNotifyRequest(_OP_NOTIFY_INVAL_INODE)

	entry := (*NotifyInvalInodeOut)(req.outData())
	entry.Ino = node
	entry.Off = off
	entry.Length = length

	return ms.notifyWrite(req)
}

func (ms *notifier) PruneNotify(nodes []uint64) Status {
	if len(nodes) == 0 {
		return OK
	}

	req := newNotifyRequest(_OP_NOTIFY_PRUNE)

	entry := (*NotifyPruneOut)(req.outData())
	entry.Count = uint32(len(nodes))

	req.outPayload = unsafe.Slice((*byte)(unsafe.Pointer(&nodes[0])), 8*len(nodes))

	return ms.notifyWrite(req)
}

// InodeNotifyStoreCache tells kernel to store data into inode's cache.
//
// This call is similar to InodeNotify, but instead of only invalidating a data
// region, it gives updated data directly to the kernel.
func (ms *notifier) InodeNotifyStoreCache(node uint64, offset int64, data []byte) Status {
	for len(data) > 0 {
		size := min(len(data),
			// NotifyStoreOut has only uint32 for size.
			// we check for max(int32), not max(uint32), because on 32-bit
			// platforms int has only 31-bit for positive range.
			math.MaxInt32)

		st := ms.inodeNotifyStoreCache32(node, offset, data[:size])
		if st != OK {
			return st
		}

		data = data[size:]
		offset += int64(size)
	}

	return OK
}

// inodeNotifyStoreCache32 is internal worker for InodeNotifyStoreCache which
// handles data chunks not larger than 2GB.
func (ms *notifier) inodeNotifyStoreCache32(node uint64, offset int64, data []byte) Status {
	req := newNotifyRequest(_OP_NOTIFY_STORE_CACHE)

	store := (*NotifyStoreOut)(req.outData())
	store.Nodeid = node
	store.Offset = uint64(offset) // NOTE not int64, as it is e.g. in NotifyInvalInodeOut
	store.Size = uint32(len(data))

	req.outPayload = data

	return ms.notifyWrite(req)
}

// InodeRetrieveCache retrieves data from kernel's inode cache.
//
// InodeRetrieveCache asks kernel to return data from its cache for inode at
// [offset:offset+len(dest)) and waits for corresponding reply. If kernel cache
// has fewer consecutive data starting at offset, that fewer amount is returned.
// In particular if inode data at offset is not cached (0, OK) is returned.
//
// The kernel returns ENOENT if it does not currently have entry for this inode
// in its dentry cache.
func (ms *notifier) InodeRetrieveCache(node uint64, offset int64, dest []byte) (n int, st Status) {
	// the kernel won't send us in one go more then what we negotiated as MaxWrite.
	// retrieve the data in chunks.
	// TODO spawn some number of readahead retrievers in parallel.
	ntotal := 0
	for {
		chunkSize := min(len(dest), ms.opts.MaxWrite)
		n, st = ms.inodeRetrieveCache1(node, offset, dest[:chunkSize])
		if st != OK || n == 0 {
			break
		}

		ntotal += n
		offset += int64(n)
		dest = dest[n:]
	}

	// if we could retrieve at least something - it is ok.
	// if ntotal=0 - st will be st returned from first inodeRetrieveCache1.
	if ntotal > 0 {
		st = OK
	}
	return ntotal, st
}

// inodeRetrieveCache1 is internal worker for InodeRetrieveCache which
// actually talks to kernel and retrieves chunks not larger than ms.opts.MaxWrite.
func (ms *notifier) inodeRetrieveCache1(node uint64, offset int64, dest []byte) (n int, st Status) {
	req := newNotifyRequest(_OP_NOTIFY_RETRIEVE_CACHE)

	// retrieve up to 2GB not to overflow uint32 size in NotifyRetrieveOut.
	// see InodeNotifyStoreCache in similar place for why it is only 2GB, not 4GB.
	//
	// ( InodeRetrieveCache calls us with chunks not larger than
	//   ms.opts.MaxWrite, but MaxWrite is int, so let's be extra cautious )
	size := min(len(dest), math.MaxInt32)
	dest = dest[:size]

	q := (*NotifyRetrieveOut)(req.outData())
	q.Nodeid = node
	q.Offset = uint64(offset) // not int64, as it is e.g. in NotifyInvalInodeOut
	q.Size = uint32(len(dest))

	reading := &retrieveCacheRequest{
		nodeid: q.Nodeid,
		offset: q.Offset,
		dest:   dest,
		ready:  make(chan struct{}),
	}

	ms.retrieveMu.Lock()
	q.NotifyUnique = ms.retrieveNext
	ms.retrieveNext++
	ms.retrieveTab[q.NotifyUnique] = reading
	ms.retrieveMu.Unlock()

	// Protect against concurrent close.
	result := ms.notifyWrite(req)
	if result != OK {
		ms.retrieveMu.Lock()
		r := ms.retrieveTab[q.NotifyUnique]
		if r == reading {
			delete(ms.retrieveTab, q.NotifyUnique)
		} else if r == nil {
			// ok - might be dequeued by umount
		} else {
			// although very unlikely, it is possible that kernel sends
			// unexpected NotifyReply with our notifyUnique, then
			// retrieveNext wraps, makes full cycle, and another
			// retrieve request is made with the same notifyUnique.
			ms.opts.Logger.Printf("W: INODE_RETRIEVE_CACHE: request with notifyUnique=%d mutated", q.NotifyUnique)
		}
		ms.retrieveMu.Unlock()
		return 0, result
	}

	// NotifyRetrieveOut sent to the kernel successfully. Now the kernel
	// have to return data in a separate write-style NotifyReply request.
	// Wait for the result.
	<-reading.ready
	return reading.n, reading.st
}

// retrieveCacheRequest represents in-flight cache retrieve request.
type retrieveCacheRequest struct {
	nodeid uint64
	offset uint64
	dest   []byte

	// reply status
	n     int
	st    Status
	ready chan struct{}
}

// DeleteNotify notifies the kernel that an entry is removed from a
// directory.  In many cases, this is equivalent to EntryNotify,
// except when the directory is in use, eg. as working directory of
// some process. You should not hold any FUSE filesystem locks, as that
// can lead to deadlock.
func (ms *notifier) DeleteNotify(parent uint64, child uint64, name string) Status {
	req := newNotifyRequest(_OP_NOTIFY_DELETE)

	entry := (*NotifyInvalDeleteOut)(req.outData())
	entry.Parent = parent
	entry.Child = child
	entry.NameLen = uint32(len(name))

	// Many versions of FUSE generate stacktraces if the
	// terminating null byte is missing.
	nameBytes := make([]byte, len(name)+1)
	copy(nameBytes, name)
	nameBytes[len(nameBytes)-1] = '\000'
	req.outPayload = nameBytes

	return ms.notifyWrite(req)
}

// EntryNotify should be used if the existence status of an entry
// within a directory changes. You should not hold any FUSE filesystem
// locks, as that can lead to deadlock.
func (ms *notifier) EntryNotify(parent uint64, name string) Status {
	req := newNotifyRequest(_OP_NOTIFY_INVAL_ENTRY)
	entry := (*NotifyInvalEntryOut)(req.outData())
	entry.Parent = parent
	entry.NameLen = uint32(len(name))

	// Many versions of FUSE generate stacktraces if the
	// terminating null byte is missing.
	nameBytes := make([]byte, len(name)+1)
	copy(nameBytes, name)
	nameBytes[len(nameBytes)-1] = '\000'
	req.outPayload = nameBytes

	return ms.notifyWrite(req)
}
