// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

const useSingleReader = false

func (ms *Server) write(req *request) Status {
	if req.outPayloadSize() == 0 {
		err := handleEINTR(func() error {
			_, err := writev(ms.mountFd, [][]byte{req.outHeaderBuf, req.outDataBuf})
			return err
		})
		return ToStatus(err)
	}
	if req.readResult != nil {
		defer req.readResult.Done()
		if ms.canSplice {
			err := ms.trySplice(req, req.readResult)
			if err == nil {
				return OK
			}
			if err != errRecoverSplice {
				ms.opts.Logger.Println("trySplice:", err)
			}
		}

		req.outPayload, req.status = req.readResult.Bytes(req.outPayload)
		req.serializeHeader(len(req.outPayload))
	}

	_, err := writev(ms.mountFd, [][]byte{req.outHeaderBuf, req.outDataBuf, req.outPayload})
	return ToStatus(err)
}
