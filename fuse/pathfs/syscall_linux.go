package pathfs

import (
	"bytes"
	"syscall"
)

func getXAttr(path string, attr string, dest []byte) (value []byte, err error) {
	sz, err := syscall.Getxattr(path, attr, dest)
	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = syscall.Getxattr(path, attr, dest)
	}

	if err != nil {
		return nil, err
	}

	return dest[:sz], err
}

func listXAttr(path string) (attributes []string, err error) {
	dest := make([]byte, 0)
	sz, err := syscall.Listxattr(path, dest)
	if err != nil {
		return nil, err
	}

	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = syscall.Listxattr(path, dest)
	}

	// -1 to drop the final empty slice.
	dest = dest[:sz-1]
	attributesBytes := bytes.Split(dest, []byte{0})
	attributes = make([]string, len(attributesBytes))
	for i, v := range attributesBytes {
		attributes[i] = string(v)
	}
	return attributes, err
}
