package fuse

import (
	"log"
	"testing"
)

func TestProtocolServerParse(t *testing.T) {
	in := [][]byte{
		[]byte("A\x00\x00\x00\x16\x00\x00\x00\x04\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00.\x04\x00\x00\x00\x00\x00\x00"),
		[]byte("\x00\x00\x00\x00\x00\x00\x00\x00security.selinux\x00"),
	}
	out := [][]byte{make([]byte, 16), make([]byte, 16)}

	opts := MountOptions{}
	opts.Debug = true
	opts.Logger = log.Default()
	ps := NewProtocolServer(NewDefaultRawFileSystem(), &opts)
	n, status := ps.HandleRequest(in, out)
	log.Println(n, status)
}
