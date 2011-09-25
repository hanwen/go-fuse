package unionfs

import (
	"exec"
	"os"
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"fmt"
	"log"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

var _ = fmt.Print
var _ = log.Print

var CheckSuccess = fuse.CheckSuccess

func setupMemUfs(t *testing.T) (workdir string, cleanup func()) {
	// Make sure system setting does not affect test.
	syscall.Umask(0)

	wd, _ := ioutil.TempDir("", "")
	err := os.Mkdir(wd+"/mount", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/backing", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)

	roFs := NewCachingFileSystem(fuse.NewLoopbackFileSystem(wd+"/ro"), 0.0)
	memFs := NewMemUnionFs(wd+"/backing", roFs)

	// We configure timeouts are smaller, so we can check for
	// UnionFs's cache consistency.
	opts := &fuse.FileSystemOptions{
		EntryTimeout:    .5 * entryTtl,
		AttrTimeout:     .5 * entryTtl,
		NegativeTimeout: .5 * entryTtl,
	}

	state, conn, err := fuse.MountNodeFileSystem(wd+"/mount", memFs, opts)
	CheckSuccess(err)
	conn.Debug = fuse.VerboseTest()
	state.Debug = fuse.VerboseTest()
	go state.Loop()

	return wd, func() {
		state.Unmount()
		os.RemoveAll(wd)
	}
}

func TestMemUnionFsSymlink(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.Symlink("/foobar", wd+"/mount/link")
	CheckSuccess(err)

	val, err := os.Readlink(wd + "/mount/link")
	CheckSuccess(err)

	if val != "/foobar" {
		t.Errorf("symlink mismatch: %v", val)
	}
}

func TestMemUnionFsSymlinkPromote(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)

	err = os.Symlink("/foobar", wd+"/mount/subdir/link")
	CheckSuccess(err)
}

func TestMemUnionFsChtimes(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "a")
	err := os.Chtimes(wd+"/ro/file", 42e9, 43e9)
	CheckSuccess(err)

	err = os.Chtimes(wd+"/mount/file", 82e9, 83e9)
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/mount/file")
	if fi.Atime_ns != 82e9 || fi.Mtime_ns != 83e9 {
		t.Error("Incorrect timestamp", fi)
	}
}

func TestMemUnionFsChmod(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	ro_fn := wd + "/ro/file"
	m_fn := wd + "/mount/file"
	writeToFile(ro_fn, "a")
	err := os.Chmod(m_fn, 07070)
	CheckSuccess(err)

	fi, err := os.Lstat(m_fn)
	CheckSuccess(err)
	if fi.Mode&07777 != 07070 {
		t.Errorf("Unexpected mode found: %o", fi.Mode)
	}
}

func TestMemUnionFsChown(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	ro_fn := wd + "/ro/file"
	m_fn := wd + "/mount/file"
	writeToFile(ro_fn, "a")

	err := os.Chown(m_fn, 0, 0)
	code := fuse.OsErrorToErrno(err)
	if code != fuse.EPERM {
		t.Error("Unexpected error code", code, err)
	}
}

func TestMemUnionFsDelete(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "a")
	_, err := os.Lstat(wd + "/mount/file")
	CheckSuccess(err)

	err = os.Remove(wd + "/mount/file")
	CheckSuccess(err)

	_, err = os.Lstat(wd + "/mount/file")
	if err == nil {
		t.Fatal("should have disappeared.")
	}
}

func TestMemUnionFsBasic(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	writeToFile(wd+"/mount/rw", "a")
	writeToFile(wd+"/ro/ro1", "a")
	writeToFile(wd+"/ro/ro2", "b")
	names := dirNames(wd + "/mount")

	expected := map[string]bool{
		"rw": true, "ro1": true, "ro2": true,
	}
	checkMapEq(t, names, expected)

	writeToFile(wd+"/mount/new", "new contents")

	contents := readFromFile(wd + "/mount/new")
	if contents != "new contents" {
		t.Errorf("read mismatch: '%v'", contents)
	}
	writeToFile(wd+"/mount/ro1", "promote me")

	remove(wd + "/mount/new")
	names = dirNames(wd + "/mount")
	checkMapEq(t, names, map[string]bool{
		"rw": true, "ro1": true, "ro2": true,
	})

	remove(wd + "/mount/ro1")
	names = dirNames(wd + "/mount")
	checkMapEq(t, names, map[string]bool{
		"rw": true, "ro2": true,
	})

}

func TestMemUnionFsPromote(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)
	writeToFile(wd+"/ro/subdir/file", "content")
	writeToFile(wd+"/mount/subdir/file", "other-content")
}

func TestMemUnionFsCreate(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/subdir/sub2", 0755)
	CheckSuccess(err)
	writeToFile(wd+"/mount/subdir/sub2/file", "other-content")
	_, err = os.Lstat(wd + "/mount/subdir/sub2/file")
	CheckSuccess(err)
}

func TestMemUnionFsOpenUndeletes(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "X")
	err := os.Remove(wd + "/mount/file")
	CheckSuccess(err)
	writeToFile(wd+"/mount/file", "X")
	_, err = os.Lstat(wd + "/mount/file")
	CheckSuccess(err)
}

func TestMemUnionFsMkdir(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	dirname := wd + "/mount/subdir"
	err := os.Mkdir(dirname, 0755)
	CheckSuccess(err)

	err = os.Remove(dirname)
	CheckSuccess(err)
}

func TestMemUnionFsMkdirPromote(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	dirname := wd + "/ro/subdir/subdir2"
	err := os.MkdirAll(dirname, 0755)
	CheckSuccess(err)

	err = os.Mkdir(wd+"/mount/subdir/subdir2/dir3", 0755)
	CheckSuccess(err)
}

func TestMemUnionFsRmdirMkdir(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)

	dirname := wd + "/mount/subdir"
	err = os.Remove(dirname)
	CheckSuccess(err)

	err = os.Mkdir(dirname, 0755)
	CheckSuccess(err)
}

func TestMemUnionFsLink(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	content := "blabla"
	fn := wd + "/ro/file"
	err := ioutil.WriteFile(fn, []byte(content), 0666)
	CheckSuccess(err)

	err = os.Link(wd+"/mount/file", wd+"/mount/linked")
	CheckSuccess(err)

	fi2, err := os.Lstat(wd + "/mount/linked")
	CheckSuccess(err)

	fi1, err := os.Lstat(wd + "/mount/file")
	CheckSuccess(err)
	if fi1.Ino != fi2.Ino {
		t.Errorf("inode numbers should be equal for linked files %v, %v", fi1.Ino, fi2.Ino)
	}
	c, err := ioutil.ReadFile(wd + "/mount/linked")
	if string(c) != content {
		t.Errorf("content mismatch got %q want %q", string(c), content)
	}
}

func TestMemUnionFsTruncate(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "hello")
	os.Truncate(wd+"/mount/file", 2)
	content := readFromFile(wd + "/mount/file")
	if content != "he" {
		t.Errorf("unexpected content %v", content)
	}
}

func TestMemUnionFsCopyChmod(t *testing.T) {
	t.Log("TestCopyChmod")
	wd, clean := setupMemUfs(t)
	defer clean()

	contents := "hello"
	fn := wd + "/mount/y"
	err := ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)

	err = os.Chmod(fn, 0755)
	CheckSuccess(err)

	fi, err := os.Lstat(fn)
	CheckSuccess(err)
	if fi.Mode&0111 == 0 {
		t.Errorf("1st attr error %o", fi.Mode)
	}
	time.Sleep(entryTtl * 1.1e9)
	fi, err = os.Lstat(fn)
	CheckSuccess(err)
	if fi.Mode&0111 == 0 {
		t.Errorf("uncached attr error %o", fi.Mode)
	}
}

func TestMemUnionFsTruncateTimestamp(t *testing.T) {
	t.Log("TestTruncateTimestamp")
	wd, clean := setupMemUfs(t)
	defer clean()

	contents := "hello"
	fn := wd + "/mount/y"
	err := ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)
	time.Sleep(0.2e9)

	truncTs := time.Nanoseconds()
	err = os.Truncate(fn, 3)
	CheckSuccess(err)

	fi, err := os.Lstat(fn)
	CheckSuccess(err)

	if abs(truncTs-fi.Mtime_ns) > 0.1e9 {
		t.Error("timestamp drift", truncTs, fi.Mtime_ns)
	}
}

func TestMemUnionFsRemoveAll(t *testing.T) {
	t.Log("TestRemoveAll")
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir/subdir", 0755)
	CheckSuccess(err)

	contents := "hello"
	fn := wd + "/ro/dir/subdir/y"
	err = ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)

	err = os.RemoveAll(wd + "/mount/dir")
	if err != nil {
		t.Error("Should delete all")
	}

	for _, f := range []string{"dir/subdir/y", "dir/subdir", "dir"} {
		if fi, _ := os.Lstat(filepath.Join(wd, "mount", f)); fi != nil {
			t.Errorf("file %s should have disappeared: %v", f, fi)
		}
	}
}

func TestMemUnionFsRmRf(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir/subdir", 0755)
	CheckSuccess(err)

	contents := "hello"
	fn := wd + "/ro/dir/subdir/y"
	err = ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)
	bin, err := exec.LookPath("rm")
	CheckSuccess(err)
	cmd := exec.Command(bin, "-rf", wd+"/mount/dir")
	err = cmd.Run()
	if err != nil {
		t.Fatal("rm -rf returned error:", err)
	}

	for _, f := range []string{"dir/subdir/y", "dir/subdir", "dir"} {
		if fi, _ := os.Lstat(filepath.Join(wd, "mount", f)); fi != nil {
			t.Errorf("file %s should have disappeared: %v", f, fi)
		}
	}
}

func TestMemUnionFsDeletedGetAttr(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/ro/file", []byte("blabla"), 0644)
	CheckSuccess(err)

	f, err := os.Open(wd + "/mount/file")
	CheckSuccess(err)
	defer f.Close()

	err = os.Remove(wd + "/mount/file")
	CheckSuccess(err)

	if fi, err := f.Stat(); err != nil || !fi.IsRegular() {
		t.Fatalf("stat returned error or non-file: %v %v", err, fi)
	}
}

func TestMemUnionFsDoubleOpen(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()
	err := ioutil.WriteFile(wd+"/ro/file", []byte("blablabla"), 0644)
	CheckSuccess(err)

	roFile, err := os.Open(wd + "/mount/file")
	CheckSuccess(err)
	defer roFile.Close()
	rwFile, err := os.OpenFile(wd+"/mount/file", os.O_WRONLY|os.O_TRUNC, 0666)
	CheckSuccess(err)
	defer rwFile.Close()

	output, err := ioutil.ReadAll(roFile)
	CheckSuccess(err)
	if len(output) != 0 {
		t.Errorf("After r/w truncation, r/o file should be empty too: %q", string(output))
	}

	want := "hello"
	_, err = rwFile.Write([]byte(want))
	CheckSuccess(err)

	b := make([]byte, 100)

	roFile.Seek(0, 0)
	n, err := roFile.Read(b)
	CheckSuccess(err)
	b = b[:n]

	if string(b) != "hello" {
		t.Errorf("r/w and r/o file are not synchronized: got %q want %q", string(b), want)
	}
}

func TestMemUnionFsFdLeak(t *testing.T) {
	beforeEntries, err := ioutil.ReadDir("/proc/self/fd")
	CheckSuccess(err)

	wd, clean := setupMemUfs(t)
	err = ioutil.WriteFile(wd+"/ro/file", []byte("blablabla"), 0644)
	CheckSuccess(err)

	contents, err := ioutil.ReadFile(wd + "/mount/file")
	CheckSuccess(err)

	err = ioutil.WriteFile(wd+"/mount/file", contents, 0644)
	CheckSuccess(err)

	clean()

	afterEntries, err := ioutil.ReadDir("/proc/self/fd")
	CheckSuccess(err)

	if len(afterEntries) != len(beforeEntries) {
		t.Errorf("/proc/self/fd changed size: after %v before %v", len(beforeEntries), len(afterEntries))
	}
}

func TestMemUnionFsStatFs(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	s1 := syscall.Statfs_t{}
	err := syscall.Statfs(wd+"/mount", &s1)
	if err != 0 {
		t.Fatal("statfs mnt", err)
	}
	if s1.Bsize == 0 {
		t.Fatal("Expect blocksize > 0")
	}
}

func TestMemUnionFsFlushSize(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	fn := wd + "/mount/file"
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE, 0644)
	CheckSuccess(err)
	fi, err := f.Stat()
	CheckSuccess(err)

	n, err := f.Write([]byte("hello"))
	CheckSuccess(err)

	f.Close()
	fi, err = os.Lstat(fn)
	CheckSuccess(err)
	if fi.Size != int64(n) {
		t.Errorf("got %d from Stat().Size, want %d", fi.Size, n)
	}
}

func TestMemUnionFsFlushRename(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/mount/file", []byte("x"), 0644)

	fn := wd + "/mount/tmp"
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE, 0644)
	CheckSuccess(err)
	fi, err := f.Stat()
	CheckSuccess(err)

	n, err := f.Write([]byte("hello"))
	CheckSuccess(err)
	f.Close()

	dst := wd + "/mount/file"
	err = os.Rename(fn, dst)
	CheckSuccess(err)

	fi, err = os.Lstat(dst)
	CheckSuccess(err)
	if fi.Size != int64(n) {
		t.Errorf("got %d from Stat().Size, want %d", fi.Size, n)
	}
}

func TestMemUnionFsTruncGetAttr(t *testing.T) {
	wd, clean := setupMemUfs(t)
	defer clean()

	c := []byte("hello")
	f, err := os.OpenFile(wd+"/mount/file", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	CheckSuccess(err)
	_, err = f.Write(c)
	CheckSuccess(err)
	err = f.Close()
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/mount/file")
	if fi.Size != int64(len(c)) {
		t.Fatalf("Length mismatch got %d want %d", fi.Size, len(c))
	}
}
