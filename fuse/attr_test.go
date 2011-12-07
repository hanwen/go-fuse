package fuse

import (
	"syscall"
	"testing"
)

func TestFileMode(t *testing.T) {
	sock := FileMode(syscall.S_IFSOCK)
	if sock.IsDir() {
		t.Error("Socket should not be directory")
	}
}
