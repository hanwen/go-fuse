//go:build !linux

package fuse

import "syscall"

// OSX and FreeBSD has races when multiple routines read
// from the FUSE device: on unmount, sometime some reads
// do not error-out, meaning that unmount will hang.
const useSingleReader = true

func (r *fuseFD) initWake(fd int) error {
	return nil
}

func (r *fuseFD) wake() {}

func (r *fuseFD) unwake() {}

func (r *fuseFD) readFD(fd int, dest []byte) (int, error) {
	return syscall.Read(fd, dest)
}

func (r *fuseFD) write(req *requestAlloc) Status {
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
			return r.writeWithSlice(&req.request, ws)
		}

		req.outPayload, req.status = req.readResult.Bytes(req.outPayload)
		req.serializeHeader(len(req.outPayload))
	}

	_, err := req.call.writev(req.outHeaderBuf, req.outDataBuf, req.outPayload)
	return ToStatus(err)
}
