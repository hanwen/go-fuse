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

func TestFilePathHash(t *testing.T) {
	// Simple test coverage.
	t.Log(filePathHash("xyz/abc"))
}

var testOpts = UnionFsOptions{
	DeletionCacheTTLSecs: entryTtl,
	DeletionDirName:      "DELETIONS",
	BranchCacheTTLSecs:   entryTtl,
}

func freezeRo(dir string) {
	err := filepath.Walk(
		dir,
		func(path string, fi *os.FileInfo, err os.Error) os.Error {
			return os.Chmod(path, (fi.Mode&0777)&^0222)
		})
	CheckSuccess(err)
}

func setupUfs(t *testing.T) (workdir string, cleanup func()) {
	// Make sure system setting does not affect test.
	syscall.Umask(0)

	wd, _ := ioutil.TempDir("", "")
	err := os.Mkdir(wd+"/mnt", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/rw", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)

	var fses []fuse.FileSystem
	fses = append(fses, fuse.NewLoopbackFileSystem(wd+"/rw"))
	fses = append(fses,
		NewCachingFileSystem(fuse.NewLoopbackFileSystem(wd+"/ro"), 0))
	ufs := NewUnionFs(fses, testOpts)

	// We configure timeouts are smaller, so we can check for
	// UnionFs's cache consistency.
	opts := &fuse.FileSystemOptions{
		EntryTimeout:    .5 * entryTtl,
		AttrTimeout:     .5 * entryTtl,
		NegativeTimeout: .5 * entryTtl,
	}

	pathfs := fuse.NewPathNodeFs(ufs,
		&fuse.PathNodeFsOptions{ClientInodes: true})
	state, conn, err := fuse.MountNodeFileSystem(wd+"/mnt", pathfs, opts)
	CheckSuccess(err)
	conn.Debug = fuse.VerboseTest()
	state.Debug = fuse.VerboseTest()
	go state.Loop()

	return wd, func() {
		state.Unmount()
		os.RemoveAll(wd)
	}
}

func writeToFile(path string, contents string) {
	err := ioutil.WriteFile(path, []byte(contents), 0644)
	CheckSuccess(err)
}

func readFromFile(path string) string {
	b, err := ioutil.ReadFile(path)
	CheckSuccess(err)
	return string(b)
}

func dirNames(path string) map[string]bool {
	f, err := os.Open(path)
	fuse.CheckSuccess(err)

	result := make(map[string]bool)
	names, err := f.Readdirnames(-1)
	fuse.CheckSuccess(err)
	err = f.Close()
	CheckSuccess(err)

	for _, nm := range names {
		result[nm] = true
	}
	return result
}

func checkMapEq(t *testing.T, m1, m2 map[string]bool) {
	if !mapEq(m1, m2) {
		msg := fmt.Sprintf("mismatch: got %v != expect %v", m1, m2)
		panic(msg)
	}
}

func mapEq(m1, m2 map[string]bool) bool {
	if len(m1) != len(m2) {
		return false
	}

	for k, v := range m1 {
		val, ok := m2[k]
		if !ok || val != v {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	f, err := os.Lstat(path)
	return err == nil && f != nil
}

func remove(path string) {
	err := os.Remove(path)
	fuse.CheckSuccess(err)
}

func TestUnionFsAutocreateDeletionDir(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Remove(wd + "/rw/DELETIONS")
	CheckSuccess(err)

	err = os.Mkdir(wd+"/mnt/dir", 0755)
	CheckSuccess(err)

	_, err = ioutil.ReadDir(wd + "/mnt/dir")
	CheckSuccess(err)
}

func TestUnionFsSymlink(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Symlink("/foobar", wd+"/mnt/link")
	CheckSuccess(err)

	val, err := os.Readlink(wd + "/mnt/link")
	CheckSuccess(err)

	if val != "/foobar" {
		t.Errorf("symlink mismatch: %v", val)
	}
}

func TestUnionFsSymlinkPromote(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)

	err = os.Symlink("/foobar", wd+"/mnt/subdir/link")
	CheckSuccess(err)
}

func TestUnionFsChtimes(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "a")
	err := os.Chtimes(wd+"/ro/file", 42e9, 43e9)
	CheckSuccess(err)

	err = os.Chtimes(wd+"/mnt/file", 82e9, 83e9)
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/mnt/file")
	if fi.Atime_ns != 82e9 || fi.Mtime_ns != 83e9 {
		t.Error("Incorrect timestamp", fi)
	}
}

func TestUnionFsChmod(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	ro_fn := wd + "/ro/file"
	m_fn := wd + "/mnt/file"
	writeToFile(ro_fn, "a")
	err := os.Chmod(m_fn, 07070)
	CheckSuccess(err)

	fi, err := os.Lstat(m_fn)
	CheckSuccess(err)
	if fi.Mode&07777 != 07270 {
		t.Errorf("Unexpected mode found: %o", fi.Mode)
	}
	_, err = os.Lstat(wd + "/rw/file")
	if err != nil {
		t.Errorf("File not promoted")
	}
}

func TestUnionFsChown(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	ro_fn := wd + "/ro/file"
	m_fn := wd + "/mnt/file"
	writeToFile(ro_fn, "a")

	err := os.Chown(m_fn, 0, 0)
	code := fuse.OsErrorToErrno(err)
	if code != fuse.EPERM {
		t.Error("Unexpected error code", code, err)
	}
}

func TestUnionFsDelete(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "a")
	_, err := os.Lstat(wd + "/mnt/file")
	CheckSuccess(err)

	err = os.Remove(wd + "/mnt/file")
	CheckSuccess(err)

	_, err = os.Lstat(wd + "/mnt/file")
	if err == nil {
		t.Fatal("should have disappeared.")
	}
	delPath := wd + "/rw/" + testOpts.DeletionDirName
	names := dirNames(delPath)
	if len(names) != 1 {
		t.Fatal("Should have 1 deletion", names)
	}

	for k, _ := range names {
		c, err := ioutil.ReadFile(delPath + "/" + k)
		CheckSuccess(err)
		if string(c) != "file" {
			t.Fatal("content mismatch", string(c))
		}
	}
}

func TestUnionFsBasic(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/rw/rw", "a")
	writeToFile(wd+"/ro/ro1", "a")
	writeToFile(wd+"/ro/ro2", "b")

	names := dirNames(wd + "/mnt")
	expected := map[string]bool{
		"rw": true, "ro1": true, "ro2": true,
	}
	checkMapEq(t, names, expected)

	writeToFile(wd+"/mnt/new", "new contents")
	if !fileExists(wd + "/rw/new") {
		t.Errorf("missing file in rw layer", names)
	}

	contents := readFromFile(wd + "/mnt/new")
	if contents != "new contents" {
		t.Errorf("read mismatch: '%v'", contents)
	}
	writeToFile(wd+"/mnt/ro1", "promote me")
	if !fileExists(wd + "/rw/ro1") {
		t.Errorf("missing file in rw layer", names)
	}

	remove(wd + "/mnt/new")
	names = dirNames(wd + "/mnt")
	checkMapEq(t, names, map[string]bool{
		"rw": true, "ro1": true, "ro2": true,
	})

	names = dirNames(wd + "/rw")
	checkMapEq(t, names, map[string]bool{
		testOpts.DeletionDirName: true,
		"rw":                     true, "ro1": true,
	})
	names = dirNames(wd + "/rw/" + testOpts.DeletionDirName)
	if len(names) != 0 {
		t.Errorf("Expected 0 entry in %v", names)
	}

	remove(wd + "/mnt/ro1")
	names = dirNames(wd + "/mnt")
	checkMapEq(t, names, map[string]bool{
		"rw": true, "ro2": true,
	})

	names = dirNames(wd + "/rw")
	checkMapEq(t, names, map[string]bool{
		"rw": true, testOpts.DeletionDirName: true,
	})

	names = dirNames(wd + "/rw/" + testOpts.DeletionDirName)
	if len(names) != 1 {
		t.Errorf("Expected 1 entry in %v", names)
	}
}

func TestUnionFsPromote(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)
	writeToFile(wd+"/ro/subdir/file", "content")
	writeToFile(wd+"/mnt/subdir/file", "other-content")
}

func TestUnionFsCreate(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/subdir/sub2", 0755)
	CheckSuccess(err)
	writeToFile(wd+"/mnt/subdir/sub2/file", "other-content")
	_, err = os.Lstat(wd + "/mnt/subdir/sub2/file")
	CheckSuccess(err)
}

func TestUnionFsOpenUndeletes(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "X")
	err := os.Remove(wd + "/mnt/file")
	CheckSuccess(err)
	writeToFile(wd+"/mnt/file", "X")
	_, err = os.Lstat(wd + "/mnt/file")
	CheckSuccess(err)
}

func TestUnionFsMkdir(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	dirname := wd + "/mnt/subdir"
	err := os.Mkdir(dirname, 0755)
	CheckSuccess(err)

	err = os.Remove(dirname)
	CheckSuccess(err)
}

func TestUnionFsMkdirPromote(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	dirname := wd + "/ro/subdir/subdir2"
	err := os.MkdirAll(dirname, 0755)
	CheckSuccess(err)

	err = os.Mkdir(wd+"/mnt/subdir/subdir2/dir3", 0755)
	CheckSuccess(err)
	fi, _ := os.Lstat(wd + "/rw/subdir/subdir2/dir3")
	CheckSuccess(err)
	if fi == nil || !fi.IsDirectory() {
		t.Error("is not a directory: ", fi)
	}
}

func TestUnionFsRmdirMkdir(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)

	dirname := wd + "/mnt/subdir"
	err = os.Remove(dirname)
	CheckSuccess(err)

	err = os.Mkdir(dirname, 0755)
	CheckSuccess(err)
}

func TestUnionFsRename(t *testing.T) {
	type Config struct {
		f1_ro bool
		f1_rw bool
		f2_ro bool
		f2_rw bool
	}

	configs := make([]Config, 0)
	for i := 0; i < 16; i++ {
		c := Config{i&0x1 != 0, i&0x2 != 0, i&0x4 != 0, i&0x8 != 0}
		if !(c.f1_ro || c.f1_rw) {
			continue
		}

		configs = append(configs, c)
	}

	for i, c := range configs {
		t.Log("Config", i, c)
		wd, clean := setupUfs(t)
		if c.f1_ro {
			writeToFile(wd+"/ro/file1", "c1")
		}
		if c.f1_rw {
			writeToFile(wd+"/rw/file1", "c2")
		}
		if c.f2_ro {
			writeToFile(wd+"/ro/file2", "c3")
		}
		if c.f2_rw {
			writeToFile(wd+"/rw/file2", "c4")
		}

		err := os.Rename(wd+"/mnt/file1", wd+"/mnt/file2")
		CheckSuccess(err)

		_, err = os.Lstat(wd + "/mnt/file1")
		if err == nil {
			t.Errorf("Should have lost file1")
		}
		_, err = os.Lstat(wd + "/mnt/file2")
		CheckSuccess(err)

		err = os.Rename(wd+"/mnt/file2", wd+"/mnt/file1")
		CheckSuccess(err)

		_, err = os.Lstat(wd + "/mnt/file2")
		if err == nil {
			t.Errorf("Should have lost file2")
		}
		_, err = os.Lstat(wd + "/mnt/file1")
		CheckSuccess(err)

		clean()
	}
}

func TestUnionFsRenameDirBasic(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir/subdir", 0755)
	CheckSuccess(err)

	err = os.Rename(wd+"/mnt/dir", wd+"/mnt/renamed")
	CheckSuccess(err)

	if fi, _ := os.Lstat(wd + "/mnt/dir"); fi != nil {
		t.Fatalf("%s/mnt/dir should have disappeared: %v", wd, fi)
	}

	if fi, _ := os.Lstat(wd + "/mnt/renamed"); fi == nil || !fi.IsDirectory() {
		t.Fatalf("%s/mnt/renamed should be directory: %v", wd, fi)
	}

	entries, err := ioutil.ReadDir(wd + "/mnt/renamed")
	if err != nil || len(entries) != 1 || entries[0].Name != "subdir" {
		t.Errorf("readdir(%s/mnt/renamed) should have one entry: %v, err %v", wd, entries, err)
	}

	if err = os.Mkdir(wd+"/mnt/dir", 0755); err != nil {
		t.Errorf("mkdir should succeed %v", err)
	}
}

func TestUnionFsRenameDirAllSourcesGone(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir", 0755)
	CheckSuccess(err)

	err = ioutil.WriteFile(wd+"/ro/dir/file.txt", []byte{42}, 0644)
	CheckSuccess(err)

	freezeRo(wd + "/ro")
	err = os.Rename(wd+"/mnt/dir", wd+"/mnt/renamed")
	CheckSuccess(err)

	names := dirNames(wd + "/rw/" + testOpts.DeletionDirName)
	if len(names) != 2 {
		t.Errorf("Expected 2 entries in %v", names)
	}
}

func TestUnionFsRenameDirWithDeletions(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir/subdir", 0755)
	CheckSuccess(err)

	err = ioutil.WriteFile(wd+"/ro/dir/file.txt", []byte{42}, 0644)
	CheckSuccess(err)

	err = ioutil.WriteFile(wd+"/ro/dir/subdir/file.txt", []byte{42}, 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	if fi, _ := os.Lstat(wd + "/mnt/dir/subdir/file.txt"); fi == nil || !fi.IsRegular() {
		t.Fatalf("%s/mnt/dir/subdir/file.txt should be file: %v", wd, fi)
	}

	err = os.Remove(wd + "/mnt/dir/file.txt")
	CheckSuccess(err)

	err = os.Rename(wd+"/mnt/dir", wd+"/mnt/renamed")
	CheckSuccess(err)

	if fi, _ := os.Lstat(wd + "/mnt/dir/subdir/file.txt"); fi != nil {
		t.Fatalf("%s/mnt/dir/subdir/file.txt should have disappeared: %v", wd, fi)
	}

	if fi, _ := os.Lstat(wd + "/mnt/dir"); fi != nil {
		t.Fatalf("%s/mnt/dir should have disappeared: %v", wd, fi)
	}

	if fi, _ := os.Lstat(wd + "/mnt/renamed"); fi == nil || !fi.IsDirectory() {
		t.Fatalf("%s/mnt/renamed should be directory: %v", wd, fi)
	}

	if fi, _ := os.Lstat(wd + "/mnt/renamed/file.txt"); fi != nil {
		t.Fatalf("%s/mnt/renamed/file.txt should have disappeared %#v", wd, fi)
	}

	if err = os.Mkdir(wd+"/mnt/dir", 0755); err != nil {
		t.Errorf("mkdir should succeed %v", err)
	}

	if fi, _ := os.Lstat(wd + "/mnt/dir/subdir"); fi != nil {
		t.Fatalf("%s/mnt/dir/subdir should have disappeared %#v", wd, fi)
	}
}

func TestUnionFsRenameSymlink(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Symlink("linktarget", wd+"/ro/link")
	CheckSuccess(err)

	err = os.Rename(wd+"/mnt/link", wd+"/mnt/renamed")
	CheckSuccess(err)

	if fi, _ := os.Lstat(wd + "/mnt/link"); fi != nil {
		t.Fatalf("%s/mnt/link should have disappeared: %v", wd, fi)
	}

	if fi, _ := os.Lstat(wd + "/mnt/renamed"); fi == nil || !fi.IsSymlink() {
		t.Fatalf("%s/mnt/renamed should be link: %v", wd, fi)
	}

	if link, err := os.Readlink(wd + "/mnt/renamed"); err != nil || link != "linktarget" {
		t.Fatalf("readlink(%s/mnt/renamed) should point to 'linktarget': %v, err %v", wd, link, err)
	}
}

func TestUnionFsWritableDir(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	dirname := wd + "/ro/subdir"
	err := os.Mkdir(dirname, 0555)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	fi, err := os.Lstat(wd + "/mnt/subdir")
	CheckSuccess(err)
	if fi.Permission()&0222 == 0 {
		t.Errorf("unexpected permission %o", fi.Permission())
	}
}

func TestUnionFsWriteAccess(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	fn := wd + "/ro/file"
	// No write perms.
	err := ioutil.WriteFile(fn, []byte("foo"), 0444)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	errno := syscall.Access(wd+"/mnt/file", fuse.W_OK)
	if errno != 0 {
		err = os.Errno(errno)
		CheckSuccess(err)
	}
}

func TestUnionFsLink(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	content := "blabla"
	fn := wd + "/ro/file"
	err := ioutil.WriteFile(fn, []byte(content), 0666)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	err = os.Link(wd+"/mnt/file", wd+"/mnt/linked")
	CheckSuccess(err)

	fi2, err := os.Lstat(wd + "/mnt/linked")
	CheckSuccess(err)

	fi1, err := os.Lstat(wd + "/mnt/file")
	CheckSuccess(err)
	if fi1.Ino != fi2.Ino {
		t.Errorf("inode numbers should be equal for linked files %v, %v", fi1.Ino, fi2.Ino)
	}
	c, err := ioutil.ReadFile(wd + "/mnt/linked")
	if string(c) != content {
		t.Errorf("content mismatch got %q want %q", string(c), content)
	}
}

func TestUnionFsTruncate(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "hello")
	freezeRo(wd + "/ro")

	os.Truncate(wd+"/mnt/file", 2)
	content := readFromFile(wd + "/mnt/file")
	if content != "he" {
		t.Errorf("unexpected content %v", content)
	}
	content2 := readFromFile(wd + "/rw/file")
	if content2 != content {
		t.Errorf("unexpected rw content %v", content2)
	}
}

func TestUnionFsCopyChmod(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	contents := "hello"
	fn := wd + "/mnt/y"
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

func abs(dt int64) int64 {
	if dt >= 0 {
		return dt
	}
	return -dt
}

func TestUnionFsTruncateTimestamp(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	contents := "hello"
	fn := wd + "/mnt/y"
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

func TestUnionFsRemoveAll(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir/subdir", 0755)
	CheckSuccess(err)

	contents := "hello"
	fn := wd + "/ro/dir/subdir/y"
	err = ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	err = os.RemoveAll(wd + "/mnt/dir")
	if err != nil {
		t.Error("Should delete all")
	}

	for _, f := range []string{"dir/subdir/y", "dir/subdir", "dir"} {
		if fi, _ := os.Lstat(filepath.Join(wd, "mount", f)); fi != nil {
			t.Errorf("file %s should have disappeared: %v", f, fi)
		}
	}

	names, err := Readdirnames(wd + "/rw/DELETIONS")
	CheckSuccess(err)
	if len(names) != 3 {
		t.Fatal("unexpected names", names)
	}
}

func TestUnionFsRmRf(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/dir/subdir", 0755)
	CheckSuccess(err)

	contents := "hello"
	fn := wd + "/ro/dir/subdir/y"
	err = ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	bin, err := exec.LookPath("rm")
	CheckSuccess(err)
	cmd := exec.Command(bin, "-rf", wd+"/mnt/dir")
	err = cmd.Run()
	if err != nil {
		t.Fatal("rm -rf returned error:", err)
	}

	for _, f := range []string{"dir/subdir/y", "dir/subdir", "dir"} {
		if fi, _ := os.Lstat(filepath.Join(wd, "mount", f)); fi != nil {
			t.Errorf("file %s should have disappeared: %v", f, fi)
		}
	}

	names, err := Readdirnames(wd + "/rw/DELETIONS")
	CheckSuccess(err)
	if len(names) != 3 {
		t.Fatal("unexpected names", names)
	}
}

func Readdirnames(dir string) ([]string, os.Error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}

	defer f.Close()
	return f.Readdirnames(-1)
}

func TestUnionFsDropDeletionCache(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/ro/file", []byte("bla"), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	_, err = os.Lstat(wd + "/mnt/file")
	CheckSuccess(err)
	err = os.Remove(wd + "/mnt/file")
	CheckSuccess(err)
	fi, _ := os.Lstat(wd + "/mnt/file")
	if fi != nil {
		t.Fatal("Lstat() should have failed", fi)
	}

	names, err := Readdirnames(wd + "/rw/DELETIONS")
	CheckSuccess(err)
	if len(names) != 1 {
		t.Fatal("unexpected names", names)
	}
	os.Remove(wd + "/rw/DELETIONS/" + names[0])
	fi, _ = os.Lstat(wd + "/mnt/file")
	if fi != nil {
		t.Fatal("Lstat() should have failed", fi)
	}

	// Expire kernel entry.
	time.Sleep(0.6e9 * entryTtl)
	err = ioutil.WriteFile(wd+"/mnt/.drop_cache", []byte(""), 0644)
	CheckSuccess(err)
	_, err = os.Lstat(wd + "/mnt/file")
	if err != nil {
		t.Fatal("Lstat() should have succeeded", err)
	}
}

func TestUnionFsDropCache(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/ro/file", []byte("bla"), 0644)
	CheckSuccess(err)

	_, err = os.Lstat(wd + "/mnt/.drop_cache")
	CheckSuccess(err)

	names, err := Readdirnames(wd + "/mnt")
	CheckSuccess(err)
	if len(names) != 1 || names[0] != "file" {
		t.Fatal("unexpected names", names)
	}

	err = ioutil.WriteFile(wd+"/ro/file2", []byte("blabla"), 0644)
	names2, err := Readdirnames(wd + "/mnt")
	CheckSuccess(err)
	if len(names2) != len(names) {
		t.Fatal("mismatch", names2)
	}

	err = ioutil.WriteFile(wd+"/mnt/.drop_cache", []byte("does not matter"), 0644)
	CheckSuccess(err)
	names2, err = Readdirnames(wd + "/mnt")
	if len(names2) != 2 {
		t.Fatal("mismatch 2", names2)
	}
}

func TestUnionFsDisappearing(t *testing.T) {
	// This init is like setupUfs, but we want access to the
	// writable Fs.
	wd, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(wd)
	err := os.Mkdir(wd+"/mnt", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/rw", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)

	wrFs := fuse.NewLoopbackFileSystem(wd + "/rw")
	var fses []fuse.FileSystem
	fses = append(fses, wrFs)
	fses = append(fses, fuse.NewLoopbackFileSystem(wd+"/ro"))
	ufs := NewUnionFs(fses, testOpts)

	opts := &fuse.FileSystemOptions{
		EntryTimeout:    entryTtl,
		AttrTimeout:     entryTtl,
		NegativeTimeout: entryTtl,
	}

	state, _, err := fuse.MountPathFileSystem(wd+"/mnt", ufs, opts)
	CheckSuccess(err)
	defer state.Unmount()
	state.Debug = fuse.VerboseTest()
	go state.Loop()

	log.Println("TestUnionFsDisappearing2")

	err = ioutil.WriteFile(wd+"/ro/file", []byte("blabla"), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	err = os.Remove(wd + "/mnt/file")
	CheckSuccess(err)

	oldRoot := wrFs.Root
	wrFs.Root = "/dev/null"
	time.Sleep(1.5 * entryTtl * 1e9)

	_, err = ioutil.ReadDir(wd + "/mnt")
	if err == nil {
		t.Fatal("Readdir should have failed")
	}
	log.Println("expected readdir failure:", err)

	err = ioutil.WriteFile(wd+"/mnt/file2", []byte("blabla"), 0644)
	if err == nil {
		t.Fatal("write should have failed")
	}
	log.Println("expected write failure:", err)

	// Restore, and wait for caches to catch up.
	wrFs.Root = oldRoot
	time.Sleep(1.5 * entryTtl * 1e9)

	_, err = ioutil.ReadDir(wd + "/mnt")
	if err != nil {
		t.Fatal("Readdir should succeed", err)
	}
	err = ioutil.WriteFile(wd+"/mnt/file2", []byte("blabla"), 0644)
	if err != nil {
		t.Fatal("write should succeed", err)
	}
}

func TestUnionFsDeletedGetAttr(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/ro/file", []byte("blabla"), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	f, err := os.Open(wd + "/mnt/file")
	CheckSuccess(err)
	defer f.Close()

	err = os.Remove(wd + "/mnt/file")
	CheckSuccess(err)

	if fi, err := f.Stat(); err != nil || !fi.IsRegular() {
		t.Fatalf("stat returned error or non-file: %v %v", err, fi)
	}
}

func TestUnionFsDoubleOpen(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()
	err := ioutil.WriteFile(wd+"/ro/file", []byte("blablabla"), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	roFile, err := os.Open(wd + "/mnt/file")
	CheckSuccess(err)
	defer roFile.Close()
	rwFile, err := os.OpenFile(wd+"/mnt/file", os.O_WRONLY|os.O_TRUNC, 0666)
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

func TestUnionFsFdLeak(t *testing.T) {
	beforeEntries, err := ioutil.ReadDir("/proc/self/fd")
	CheckSuccess(err)

	wd, clean := setupUfs(t)
	err = ioutil.WriteFile(wd+"/ro/file", []byte("blablabla"), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	contents, err := ioutil.ReadFile(wd + "/mnt/file")
	CheckSuccess(err)

	err = ioutil.WriteFile(wd+"/mnt/file", contents, 0644)
	CheckSuccess(err)

	clean()

	afterEntries, err := ioutil.ReadDir("/proc/self/fd")
	CheckSuccess(err)

	if len(afterEntries) != len(beforeEntries) {
		t.Errorf("/proc/self/fd changed size: after %v before %v", len(beforeEntries), len(afterEntries))
	}
}

func TestUnionFsStatFs(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	s1 := syscall.Statfs_t{}
	err := syscall.Statfs(wd+"/mnt", &s1)
	if err != 0 {
		t.Fatal("statfs mnt", err)
	}
	if s1.Bsize == 0 {
		t.Fatal("Expect blocksize > 0")
	}
}

func TestUnionFsFlushSize(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	fn := wd + "/mnt/file"
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

func TestUnionFsFlushRename(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/mnt/file", []byte("x"), 0644)

	fn := wd + "/mnt/tmp"
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE, 0644)
	CheckSuccess(err)
	fi, err := f.Stat()
	CheckSuccess(err)

	n, err := f.Write([]byte("hello"))
	CheckSuccess(err)
	f.Close()

	dst := wd + "/mnt/file"
	err = os.Rename(fn, dst)
	CheckSuccess(err)

	fi, err = os.Lstat(dst)
	CheckSuccess(err)
	if fi.Size != int64(n) {
		t.Errorf("got %d from Stat().Size, want %d", fi.Size, n)
	}
}

func TestUnionFsTruncGetAttr(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	c := []byte("hello")
	f, err := os.OpenFile(wd+"/mnt/file", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	CheckSuccess(err)
	_, err = f.Write(c)
	CheckSuccess(err)
	err = f.Close()
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/mnt/file")
	if fi.Size != int64(len(c)) {
		t.Fatalf("Length mismatch got %d want %d", fi.Size, len(c))
	}
}

func TestUnionFsPromoteDirTimeStamp(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0750)
	CheckSuccess(err)
	err = ioutil.WriteFile(wd+"/ro/subdir/file", []byte("hello"), 0644)
	CheckSuccess(err)
	freezeRo(wd + "/ro")

	err = os.Chmod(wd+"/mnt/subdir/file", 0060)
	CheckSuccess(err)

	fRo, err := os.Lstat(wd + "/ro/subdir")
	CheckSuccess(err)
	fRw, err := os.Lstat(wd + "/rw/subdir")
	CheckSuccess(err)

	// TODO - need to update timestamps after promoteDirsTo calls,
	// not during.
	if false && fRo.Mtime_ns != fRw.Mtime_ns {
		t.Errorf("Changed timestamps on promoted subdir: ro %d rw %d", fRo.Mtime_ns, fRw.Mtime_ns)
	}

	if fRo.Mode|0200 != fRw.Mode {
		t.Errorf("Changed mode ro: %o, rw: %o", fRo.Mode, fRw.Mode)
	}
}
