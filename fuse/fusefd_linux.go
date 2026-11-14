// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"encoding/binary"
	"syscall"

	"golang.org/x/sys/unix"
)

const useSingleReader = false

// initWake makes fd non-blocking, so readers can wait for a request
// in epoll_wait(2) together with wakeFD.
func (f *fuseFD) initWake(fd int) error {
	efd, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		return err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(efd)
		return err
	}
	f.wakeFD = efd
	return nil
}

// wake makes wakeFD readable. It stays readable, so it wakes all
// current and future readers. Called with reqMu held.
func (f *fuseFD) wake() {
	if f.wakeFD < 0 {
		return
	}
	var one [8]byte
	binary.NativeEndian.PutUint64(one[:], 1)
	syscall.Write(f.wakeFD, one[:])
}

// unwake undoes wake. Called with reqMu held.
func (f *fuseFD) unwake() {
	if f.wakeFD < 0 {
		return
	}
	var buf [8]byte
	syscall.Read(f.wakeFD, buf[:])
}

// readFD reads a request from fd, which is non-blocking. If no request
// is queued, the reader waits in epoll_wait(2), on an epoll fd of its
// own. The FUSE fd is added with EPOLLEXCLUSIVE, so a request wakes
// one reader, like a blocking read(2) does. It returns errStopped if
// wakeFD becomes readable while no request is queued.
func (f *fuseFD) readFD(fd int, dest []byte) (int, error) {
	n, err := syscall.Read(fd, dest)
	if err != syscall.EAGAIN {
		return n, err
	}
	epfd, err := f.getEpoll(fd)
	if err != nil {
		return 0, err
	}
	defer f.putEpoll(epfd)
	for {
		var evs [2]unix.EpollEvent
		k, err := unix.EpollWait(epfd, evs[:], -1)
		if err != nil && err != unix.EINTR {
			return 0, err
		}
		for _, ev := range evs[:max(k, 0)] {
			if ev.Fd == int32(f.wakeFD) {
				return 0, errStopped
			}
		}
		n, err := syscall.Read(fd, dest)
		if err != syscall.EAGAIN {
			return n, err
		}
	}
}

func (f *fuseFD) getEpoll(fd int) (int, error) {
	f.reqMu.Lock()
	defer f.reqMu.Unlock()
	if k := len(f.epollFDs); k > 0 {
		epfd := f.epollFDs[k-1]
		f.epollFDs = f.epollFDs[:k-1]
		return epfd, nil
	}

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return -1, err
	}
	for _, ev := range []unix.EpollEvent{
		{Events: unix.EPOLLIN | unix.EPOLLEXCLUSIVE, Fd: int32(fd)},
		{Events: unix.EPOLLIN, Fd: int32(f.wakeFD)},
	} {
		if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, int(ev.Fd), &ev); err != nil {
			syscall.Close(epfd)
			return -1, err
		}
	}
	return epfd, nil
}

func (f *fuseFD) putEpoll(epfd int) {
	f.reqMu.Lock()
	f.epollFDs = append(f.epollFDs, epfd)
	f.reqMu.Unlock()
}

func (f *fuseFD) write(req *requestAlloc) Status {
	if req.outPayloadSize() == 0 {
		_, err := req.call.writev(req.outHeaderBuf, req.outDataBuf)
		return ToStatus(err)
	}

	if req.readResult != nil {
		defer func() {
			req.readResult.Done()
			req.readResult = nil
		}()
		if ws, ok := req.readResult.(withSlice); ok {
			return f.writeWithSlice(&req.request, ws)
		}

		if f.canSplice {
			err := f.trySplice(&req.request, req.readResult)
			if err == nil {
				return OK
			}
			if err != errRecoverSplice {
				f.logger.Println("trySplice:", err)
			}
		}

		req.outPayload, req.status = req.readResult.Bytes(req.outPayload)
		req.serializeHeader(len(req.outPayload))
	}

	_, err := req.call.writev(req.outHeaderBuf, req.outDataBuf, req.outPayload)
	return ToStatus(err)
}
