//go:build freebsd || openbsd

package fuse

func (s *Server) setSplice() {
	s.fuseFD.canSplice = false
}

func (s *Server) trySplice(header []byte, req *request, fdData *readResultFd) error {
	return errRecoverSplice
}
