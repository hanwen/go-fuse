package pathfs

import (
	"bytes"
	"syscall"
	"unsafe"
)

var _zero uintptr

func getXAttr(path string, attr string, dest []byte) (value []byte, err error) {
	sz, err := sysGetxattr(path, attr, dest)
	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = sysGetxattr(path, attr, dest)
	}

	if err != nil {
		return nil, err
	}

	return dest[:sz], err
}

func listXAttr(path string) (attributes []string, err error) {
	dest := make([]byte, 0)
	sz, err := sysListxattr(path, dest)
	if err != nil {
		return nil, err
	}

	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = sysListxattr(path, dest)
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

// Below is cut & paste from std lib syscall, so gccgo 4.8.1 can
// compile this too.
func sysGetxattr(path string, attr string, dest []byte) (sz int, err error) {
	var _p0 *byte
	_p0, err = syscall.BytePtrFromString(path)
	if err != nil {
		return
	}
	var _p1 *byte
	_p1, err = syscall.BytePtrFromString(attr)
	if err != nil {
		return
	}
	var _p2 unsafe.Pointer
	if len(dest) > 0 {
		_p2 = unsafe.Pointer(&dest[0])
	} else {
		_p2 = unsafe.Pointer(&_zero)
	}
	r0, _, e1 := syscall.Syscall6(syscall.SYS_GETXATTR, uintptr(unsafe.Pointer(_p0)), uintptr(unsafe.Pointer(_p1)), uintptr(_p2), uintptr(len(dest)), 0, 0)
	sz = int(r0)
	if e1 != 0 {
		err = e1
	}
	return
}

func sysRemovexattr(path string, attr string) (err error) {
	var _p0 *byte
	_p0, err = syscall.BytePtrFromString(path)
	if err != nil {
		return
	}
	var _p1 *byte
	_p1, err = syscall.BytePtrFromString(attr)
	if err != nil {
		return
	}
	_, _, e1 := syscall.Syscall(syscall.SYS_REMOVEXATTR, uintptr(unsafe.Pointer(_p0)), uintptr(unsafe.Pointer(_p1)), 0)
	if e1 != 0 {
		err = e1
	}
	return
}

func sysListxattr(path string, dest []byte) (sz int, err error) {
	var _p0 *byte
	_p0, err = syscall.BytePtrFromString(path)
	if err != nil {
		return
	}
	var _p1 unsafe.Pointer
	if len(dest) > 0 {
		_p1 = unsafe.Pointer(&dest[0])
	} else {
		_p1 = unsafe.Pointer(&_zero)
	}
	r0, _, e1 := syscall.Syscall(syscall.SYS_LISTXATTR, uintptr(unsafe.Pointer(_p0)), uintptr(_p1), uintptr(len(dest)))
	sz = int(r0)
	if e1 != 0 {
		err = e1
	}
	return
}

func sysSetxattr(path string, attr string, val []byte, flag int) error {
	return syscall.Setxattr(path, attr, val, flag)
}
