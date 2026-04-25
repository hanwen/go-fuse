package posixtest

import (
	"bytes"
	"io"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func sysFcntlFlockGetOFDLock(fd uintptr, lk *syscall.Flock_t) error {
	return syscall.FcntlFlock(fd, unix.F_OFD_GETLK, lk)
}

func FallocateKeepSize(t *testing.T, mnt string) {
	f, err := os.Create(mnt + "/file")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data := bytes.Repeat([]byte{42}, 100)
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Fallocate(int(f.Fd()), unix.FALLOC_FL_KEEP_SIZE, 50, 52); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	roundtrip, _ := io.ReadAll(f)
	if !bytes.Equal(roundtrip, data) {
		t.Fatalf("roundtrip not equal %q != %q", roundtrip, data)
	}
}

func init() {
	All["FallocateKeepSize"] = FallocateKeepSize
}
