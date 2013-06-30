package fuse

import (
	"fmt"
	"io"

	"github.com/hanwen/go-fuse/splice"
)

func (s *Server) setSplice() {
	s.canSplice = splice.Resizable()
}

func (ms *Server) trySplice(header []byte, req *request, fdData *readResultFd) error {
	pair, err := splice.Get()
	if err != nil {
		return err
	}
	defer splice.Done(pair)

	total := len(header) + fdData.Size()
	if err := pair.Grow(total); err != nil {
		return err
	}

	_, err = pair.Write(header)
	if err != nil {
		return err
	}

	var n int
	if fdData.Off < 0 {
		n, err = pair.LoadFrom(fdData.Fd, fdData.Size())
	} else {
		n, err = pair.LoadFromAt(fdData.Fd, fdData.Size(), fdData.Off)
	}
	if err == io.EOF || (err == nil && n < fdData.Size()) {
		discard := make([]byte, len(header))
		_, err = pair.Read(discard)
		if err != nil {
			return err
		}

		header = req.serializeHeader(n)

		newFd := readResultFd{
			Fd:  pair.ReadFd(),
			Off: -1,
			Sz:  n,
		}
		return ms.trySplice(header, req, &newFd)
	}

	if err != nil {
		// TODO - extract the data from splice.
		return err
	}

	if n != fdData.Size() {
		return fmt.Errorf("wrote %d, want %d", n, fdData.Size())
	}

	_, err = pair.WriteTo(uintptr(ms.mountFd), total)
	if err != nil {
		return err
	}
	return nil
}
