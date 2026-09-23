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

// Tmpfile checks that a file created with O_TMPFILE can be written and read back, and that it is in no directory.
func Tmpfile(t *testing.T, mnt string) {
	fd, err := syscall.Open(mnt, unix.O_TMPFILE|syscall.O_RDWR, 0o600)
	// A kernel which does not know O_TMPFILE opens the directory itself, which fails with EISDIR.
	if err == syscall.EOPNOTSUPP || err == syscall.EISDIR {
		t.Skipf("FS does not support O_TMPFILE: %v", err)
	}
	if err != nil {
		t.Fatalf("O_TMPFILE: %v", err)
	}
	defer syscall.Close(fd)

	want := "temporary"
	if _, err := syscall.Pwrite(fd, []byte(want), 0); err != nil {
		t.Fatalf("Pwrite: %v", err)
	}
	got := make([]byte, len(want))
	if _, err := syscall.Pread(fd, got, 0); err != nil {
		t.Fatalf("Pread: %v", err)
	}
	if string(got) != want {
		t.Errorf("read %q, want %q", got, want)
	}

	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		t.Fatalf("Fstat: %v", err)
	}
	if st.Nlink != 0 {
		t.Errorf("Nlink is %d, want 0: the file is in no directory", st.Nlink)
	}

	entries, err := os.ReadDir(mnt)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("%s has %d entries, want none", mnt, len(entries))
	}
}

func init() {
	All["FallocateKeepSize"] = FallocateKeepSize
	All["Tmpfile"] = Tmpfile
}
