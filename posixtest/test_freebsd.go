package posixtest

import (
	"syscall"
	"unsafe"
)

// Since FreeBSD doesn't implement the F_OFD_GETLK, to mimic its behaviour,
// we here first fork the process via syscall, and execute fcntl(2) in the child
// process to get the lock info. Then we send the lock info via pipe back to
// the parent test process.

func sysFcntlFlockGetOFDLock(fd uintptr, lk *syscall.Flock_t) error {
	pipefd := make([]int, 2)
	err := syscall.Pipe(pipefd)
	if err != nil {
		return err
	}
	pid, _, err := syscall.Syscall(syscall.SYS_FORK, 0, 0, 0)
	if pid == 0 { // child process
		syscall.Close(pipefd[0]) // close read end
		var clk syscall.Flock_t
		// Here we must give a vaild lock type, or fcntl(2) will return
		// EINVAL. And it should be different from what we set earlier
		// in the test. Here we set to F_RDLOCK.
		clk.Type = syscall.F_RDLCK
		syscall.FcntlFlock(fd, syscall.F_GETLK, &clk)
		syscall.Syscall(syscall.SYS_WRITE, uintptr(pipefd[1]), uintptr(unsafe.Pointer(&clk)), unsafe.Sizeof(clk))
		syscall.Exit(0)
	} else if pid > 0 { // parent process
		syscall.Close(pipefd[1]) // close write end
		buf := make([]byte, unsafe.Sizeof(*lk))
		syscall.Read(pipefd[0], buf)
		*lk = *((*syscall.Flock_t)(unsafe.Pointer(&buf[0])))
		syscall.Close(pipefd[0]) // close read end
	} else {
		return err
	}
	return nil
}
