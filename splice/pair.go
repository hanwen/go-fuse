package splice

import (
	"fmt"
	"os"
	"syscall"
)

type Pair struct {
	r, w *os.File
	size int
}

func (p *Pair) MaxGrow() {
	for p.Grow(2 * p.size) {
	}
}

func (p *Pair) Grow(n int) bool {
	if n <= p.size {
		return true
	}
	if !resizable {
		return false
	}
	if n > maxPipeSize {
		return false
	}

	newsize, errNo := fcntl(p.r.Fd(), F_SETPIPE_SZ, n)
	if errNo != 0 {
		return false
	}
	p.size = newsize
	return true
}

func (p *Pair) Cap() int {
	return p.size
}

func (p *Pair) Close() error {
	err1 := p.r.Close()
	err2 := p.w.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (p *Pair) Read(d []byte) (n int, err error) {
	return p.r.Read(d)
}

func (p *Pair) ReadFd() uintptr {
	return p.r.Fd()
}

func (p *Pair) WriteFd() uintptr {
	return p.w.Fd()
}

func (p *Pair) Write(d []byte) (n int, err error) {
	return p.w.Write(d)
}

func (p *Pair) LoadFromAt(fd uintptr, sz int, off int64) (int, error) {
	n, err := syscall.Splice(int(fd), &off, int(p.w.Fd()), nil, sz, 0)
	return int(n), err
}

func (p *Pair) LoadFrom(fd uintptr, sz int) (int, error) {
	if sz > p.size {
		return 0, fmt.Errorf("LoadFrom: not enough space %d, %d",
			sz, p.size)
	}

	n, err := syscall.Splice(int(fd), nil, int(p.w.Fd()), nil, sz, 0)
	if err != nil {
		err = os.NewSyscallError("Splice load from", err)
	}
	return int(n), err
}

func (p *Pair) WriteTo(fd uintptr, n int) (int, error) {
	m, err := syscall.Splice(int(p.r.Fd()), nil, int(fd), nil, int(n), 0)
	if err != nil {
		err = os.NewSyscallError("Splice write", err)
	}
	return int(m), err
}
