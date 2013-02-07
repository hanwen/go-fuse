package fuse

import (
	"fmt"
)

func (s *MountState) setSplice() {
	panic("darwin has no splice.")
}

func (ms *MountState) trySplice(header []byte, req *request, fdData *ReadResultFd) error {
	return fmt.Errorf("unimplemented")
}
