// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
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

	// maxWrite is the negotiated MaxWrite, which bounds
	// NOTIFY_RETRIEVE chunks. Set before OnConnect.
	maxWrite int

	retrieveMu   sync.Mutex
	retrieveNext uint64
	retrieveTab  map[uint64]*retrieveCacheRequest // notifyUnique -> retrieve request

	// retrieveIdle is closed when retrieveTab becomes empty.
	retrieveIdle chan struct{}
}

// deleteRetrieveLocked removes a retrieve from the table.
func (n *notifier) deleteRetrieveLocked(unique uint64) {
	delete(n.retrieveTab, unique)
	if len(n.retrieveTab) == 0 && n.retrieveIdle != nil {
		close(n.retrieveIdle)
		n.retrieveIdle = nil
	}
}

// waitRetrieves waits until no retrieve waits for a reply.
func (n *notifier) waitRetrieves(ctx context.Context) error {
	n.retrieveMu.Lock()
	if len(n.retrieveTab) == 0 {
		n.retrieveMu.Unlock()
		return nil
	}
	if n.retrieveIdle == nil {
		n.retrieveIdle = make(chan struct{})
	}
	ch := n.retrieveIdle
	n.retrieveMu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// newNotify returns a Notify that sends notifications by calling
// writev with the serialized message. Replies to NOTIFY_RETRIEVE are
// delivered by the server that the Notify is installed in.
func newNotify(writev func([][]byte) (int, syscall.Errno), opts *MountOptions) *Notify {
	n := &notifier{
		writev:      writev,
		opts:        opts,
		maxWrite:    opts.MaxWrite,
		retrieveTab: make(map[uint64]*retrieveCacheRequest),
	}
	return &Notify{
		InvalInode: n.InodeNotify,
		InvalEntry: n.EntryNotify,
		Delete:     n.DeleteNotify,
		Prune:      n.PruneNotify,
		Store:      n.InodeNotifyStoreCache,
		Retrieve:   n.InodeRetrieveCache,
		notifier:   n,
	}
}

// deliverReply hands a NOTIFY_REPLY from the kernel to the waiting
// Retrieve call.
func (n *notifier) deliverReply(reply *NotifyRetrieveIn, payload []byte) {
	n.retrieveMu.Lock()
	reading := n.retrieveTab[reply.Unique]
	n.deleteRetrieveLocked(reply.Unique)
	n.retrieveMu.Unlock()

	badf := func(format string, argv ...any) {
		n.opts.Logger.Printf("notify reply: "+format, argv...)
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
func (n *notifier) abort() {
	// It is possible that umount comes in the middle - after retrieve
	// request was sent to kernel, but corresponding kernel reply has not
	// yet been read. We unblock all such readers and wake them up with ENODEV.
	n.retrieveMu.Lock()
	rtab := n.retrieveTab
	// retrieve attempts might be erroneously tried even after close
	// we have to keep retrieveTab !nil not to panic.
	n.retrieveTab = make(map[uint64]*retrieveCacheRequest)
	if n.retrieveIdle != nil {
		close(n.retrieveIdle)
		n.retrieveIdle = nil
	}
	n.retrieveMu.Unlock()
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

func (n *notifier) notifyWrite(req *request) Status {
	return notifyWrite(n.writev, n.opts, req)
}

func newNotifyRequest(opcode uint32) *request {
	r := &request{
		inputBuf:     make([]byte, unsafe.Sizeof(InHeader{})),
		outHeaderBuf: make([]byte, sizeOfOutHeader),
		outDataBuf:   make([]byte, getHandler(opcode).OutputSize),
		notifyCode: map[uint32]notifyCode{
			_OP_NOTIFY_INVAL_INODE:    notifyInvalInode,
			_OP_NOTIFY_INVAL_ENTRY:    notifyInvalEntry,
			_OP_NOTIFY_STORE_CACHE:    notifyStore,
			_OP_NOTIFY_RETRIEVE_CACHE: notifyRetrieve,
			_OP_NOTIFY_DELETE:         notifyDelete,
			_OP_NOTIFY_PRUNE:          notifyPrune,
		}[opcode],
	}
	r.inHeader().Opcode = opcode
	return r
}

// InodeNotify invalidates the information associated with the inode
// (ie. data cache, attributes, etc.)
func (n *notifier) InodeNotify(node uint64, off int64, length int64) Status {
	req := newNotifyRequest(_OP_NOTIFY_INVAL_INODE)

	entry := (*NotifyInvalInodeOut)(req.outData())
	entry.Ino = node
	entry.Off = off
	entry.Length = length

	return n.notifyWrite(req)
}

func (n *notifier) PruneNotify(nodes []uint64) Status {
	if len(nodes) == 0 {
		return OK
	}

	req := newNotifyRequest(_OP_NOTIFY_PRUNE)

	entry := (*NotifyPruneOut)(req.outData())
	entry.Count = uint32(len(nodes))

	req.outPayload = unsafe.Slice((*byte)(unsafe.Pointer(&nodes[0])), 8*len(nodes))

	return n.notifyWrite(req)
}

// InodeNotifyStoreCache tells kernel to store data into inode's cache.
//
// This call is similar to InodeNotify, but instead of only invalidating a data
// region, it gives updated data directly to the kernel.
func (n *notifier) InodeNotifyStoreCache(node uint64, offset int64, data []byte) Status {
	for len(data) > 0 {
		size := min(len(data),
			// NotifyStoreOut has only uint32 for size.
			// we check for max(int32), not max(uint32), because on 32-bit
			// platforms int has only 31-bit for positive range.
			math.MaxInt32)

		st := n.inodeNotifyStoreCache32(node, offset, data[:size])
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
func (n *notifier) inodeNotifyStoreCache32(node uint64, offset int64, data []byte) Status {
	req := newNotifyRequest(_OP_NOTIFY_STORE_CACHE)

	store := (*NotifyStoreOut)(req.outData())
	store.Nodeid = node
	store.Offset = uint64(offset) // NOTE not int64, as it is e.g. in NotifyInvalInodeOut
	store.Size = uint32(len(data))

	req.outPayload = data

	return n.notifyWrite(req)
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
func (n *notifier) InodeRetrieveCache(node uint64, offset int64, dest []byte) (int, Status) {
	// the kernel won't send us in one go more then what we negotiated as MaxWrite.
	// retrieve the data in chunks.
	// TODO spawn some number of readahead retrievers in parallel.
	ntotal := 0
	var st Status
	for {
		chunkSize := min(len(dest), n.maxWrite)
		var got int
		got, st = n.inodeRetrieveCache1(node, offset, dest[:chunkSize])
		if st != OK || got == 0 {
			break
		}

		ntotal += got
		offset += int64(got)
		dest = dest[got:]
	}

	// if we could retrieve at least something - it is ok.
	// if ntotal=0 - st will be st returned from first inodeRetrieveCache1.
	if ntotal > 0 {
		st = OK
	}
	return ntotal, st
}

// inodeRetrieveCache1 is internal worker for InodeRetrieveCache which
// actually talks to kernel and retrieves chunks not larger than the negotiated MaxWrite.
func (n *notifier) inodeRetrieveCache1(node uint64, offset int64, dest []byte) (int, Status) {
	req := newNotifyRequest(_OP_NOTIFY_RETRIEVE_CACHE)

	// retrieve up to 2GB not to overflow uint32 size in NotifyRetrieveOut.
	// see InodeNotifyStoreCache in similar place for why it is only 2GB, not 4GB.
	//
	// ( InodeRetrieveCache calls us with chunks not larger than
	//   MaxWrite, but let's be extra cautious )
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

	n.retrieveMu.Lock()
	q.NotifyUnique = n.retrieveNext
	n.retrieveNext++
	n.retrieveTab[q.NotifyUnique] = reading
	n.retrieveMu.Unlock()

	// Protect against concurrent close.
	result := n.notifyWrite(req)
	if result != OK {
		n.retrieveMu.Lock()
		r := n.retrieveTab[q.NotifyUnique]
		if r == reading {
			n.deleteRetrieveLocked(q.NotifyUnique)
		} else if r == nil {
			// ok - might be dequeued by umount
		} else {
			// although very unlikely, it is possible that kernel sends
			// unexpected NotifyReply with our notifyUnique, then
			// retrieveNext wraps, makes full cycle, and another
			// retrieve request is made with the same notifyUnique.
			n.opts.Logger.Printf("W: INODE_RETRIEVE_CACHE: request with notifyUnique=%d mutated", q.NotifyUnique)
		}
		n.retrieveMu.Unlock()
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
func (n *notifier) DeleteNotify(parent uint64, child uint64, name string) Status {
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

	return n.notifyWrite(req)
}

// EntryNotify should be used if the existence status of an entry
// within a directory changes. You should not hold any FUSE filesystem
// locks, as that can lead to deadlock.
func (n *notifier) EntryNotify(parent uint64, name string) Status {
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

	return n.notifyWrite(req)
}
