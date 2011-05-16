package unionfs

import (
	"os"
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"fmt"
	"log"
	"testing"
	"time"
)

var _ = fmt.Print
var _ = log.Print

var CheckSuccess = fuse.CheckSuccess

func TestFilePathHash(t *testing.T) {
	// Simple test coverage.
	t.Log(filePathHash("xyz/abc"))
}

var testOpts = UnionFsOptions{
	DeletionCacheTTLSecs: entryTtl,
	DeletionDirName:      "DELETIONS",
	BranchCacheTTLSecs:   entryTtl,
}

func setupUfs(t *testing.T) (workdir string, cleanup func()) {
	wd := fuse.MakeTempDir()
	err := os.Mkdir(wd+"/mount", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/rw", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)

	var fses []fuse.FileSystem
	fses = append(fses, fuse.NewLoopbackFileSystem(wd+"/rw"))
	fses = append(fses, fuse.NewLoopbackFileSystem(wd+"/ro"))
	ufs := NewUnionFs("testFs", fses, testOpts)

	opts := &fuse.FileSystemOptions{
		EntryTimeout:    entryTtl,
		AttrTimeout:     entryTtl,
		NegativeTimeout: entryTtl,
	}

	state, _, err := fuse.MountFileSystem(wd + "/mount", ufs, opts)
	CheckSuccess(err)
	state.Debug = true
	go state.Loop(false)

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
	fmt.Println(b)
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
		log.Print(msg)
		t.Error(msg)
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

func TestSymlink(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Symlink("/foobar", wd+"/mount/link")
	CheckSuccess(err)

	val, err := os.Readlink(wd + "/mount/link")
	CheckSuccess(err)

	if val != "/foobar" {
		t.Errorf("symlink mismatch: %v", val)
	}
}

func TestChtimes(t *testing.T) {
	wd, clean := setupUfs(t)
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

func TestChmod(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	ro_fn := wd + "/ro/file"
	m_fn := wd + "/mount/file"
	writeToFile(ro_fn, "a")
	err := os.Chmod(m_fn, 07070)
	CheckSuccess(err)

	err = os.Chown(m_fn, 0, 0)
	code := fuse.OsErrorToErrno(err)
	if code != fuse.EPERM {
		t.Error("Unexpected error code", code, err)
	}

	fi, err := os.Lstat(m_fn)
	CheckSuccess(err)
	if fi.Mode&07777 != 07272 {
		t.Errorf("Unexpected mode found: %o", fi.Mode)
	}
	_, err = os.Lstat(wd + "/rw/file")
	if err != nil {
		t.Errorf("File not promoted")
	}
}

func TestBasic(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/rw/rw", "a")
	writeToFile(wd+"/ro/ro1", "a")
	writeToFile(wd+"/ro/ro2", "b")

	names := dirNames(wd + "/mount")
	expected := map[string]bool{
		"rw": true, "ro1": true, "ro2": true,
	}
	checkMapEq(t, names, expected)

	log.Println("new contents")
	writeToFile(wd+"/mount/new", "new contents")
	if !fileExists(wd + "/rw/new") {
		t.Errorf("missing file in rw layer", names)
	}

	contents := readFromFile(wd + "/mount/new")
	if contents != "new contents" {
		t.Errorf("read mismatch: '%v'", contents)
	}
	return
	writeToFile(wd+"/mount/ro1", "promote me")
	if !fileExists(wd + "/rw/ro1") {
		t.Errorf("missing file in rw layer", names)
	}

	remove(wd + "/mount/new")
	names = dirNames(wd + "/mount")
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

	remove(wd + "/mount/ro1")
	names = dirNames(wd + "/mount")
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

func TestPromote(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Mkdir(wd+"/ro/subdir", 0755)
	CheckSuccess(err)
	writeToFile(wd+"/ro/subdir/file", "content")
	writeToFile(wd+"/mount/subdir/file", "other-content")
}

func TestCreate(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	err := os.MkdirAll(wd+"/ro/subdir/sub2", 0755)
	CheckSuccess(err)
	writeToFile(wd+"/mount/subdir/sub2/file", "other-content")
	_, err = os.Lstat(wd + "/mount/subdir/sub2/file")
	CheckSuccess(err)
}

func TestOpenUndeletes(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "X")
	err := os.Remove(wd + "/mount/file")
	CheckSuccess(err)
	writeToFile(wd+"/mount/file", "X")
	_, err = os.Lstat(wd + "/mount/file")
	CheckSuccess(err)
}

func TestMkdir(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	dirname := wd + "/mount/subdir"
	err := os.Mkdir(dirname, 0755)
	CheckSuccess(err)

	err = os.Remove(dirname)
	CheckSuccess(err)
}

func TestMkdirPromote(t *testing.T) {
	wd, clean := setupUfs(t)
	defer clean()

	dirname := wd + "/ro/subdir/subdir2"
	err := os.MkdirAll(dirname, 0755)
	CheckSuccess(err)

	err = os.Mkdir(wd+"/mount/subdir/subdir2/dir3", 0755)
	CheckSuccess(err)
	fi, _ := os.Lstat(wd + "/rw/subdir/subdir2/dir3")
	CheckSuccess(err)
	if fi == nil || !fi.IsDirectory() {
		t.Error("is not a directory: ", fi)
	}
}
func TestRename(t *testing.T) {
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

		err := os.Rename(wd+"/mount/file1", wd+"/mount/file2")
		CheckSuccess(err)

		_, err = os.Lstat(wd + "/mount/file1")
		if err == nil {
			t.Errorf("Should have lost file1")
		}
		_, err = os.Lstat(wd + "/mount/file2")
		CheckSuccess(err)

		err = os.Rename(wd+"/mount/file2", wd+"/mount/file1")
		CheckSuccess(err)

		_, err = os.Lstat(wd + "/mount/file2")
		if err == nil {
			t.Errorf("Should have lost file2")
		}
		_, err = os.Lstat(wd + "/mount/file1")
		CheckSuccess(err)

		clean()
	}
}

func TestWritableDir(t *testing.T) {
	t.Log("TestWritableDir")
	wd, clean := setupUfs(t)
	defer clean()

	dirname := wd + "/ro/subdir"
	err := os.Mkdir(dirname, 0555)
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/mount/subdir")
	CheckSuccess(err)
	if fi.Permission()&0222 == 0 {
		t.Errorf("unexpected permission %o", fi.Permission())
	}
}

func TestTruncate(t *testing.T) {
	t.Log("TestTruncate")
	wd, clean := setupUfs(t)
	defer clean()

	writeToFile(wd+"/ro/file", "hello")
	os.Truncate(wd+"/mount/file", 2)
	content := readFromFile(wd + "/mount/file")
	if content != "he" {
		t.Errorf("unexpected content %v", content)
	}
	content2 := readFromFile(wd + "/rw/file")
	if content2 != content {
		t.Errorf("unexpected rw content %v", content2)
	}
}

func TestCopyChmod(t *testing.T) {
	t.Log("TestCopyChmod")
	wd, clean := setupUfs(t)
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

func abs(dt int64) int64 {
	if dt >= 0 {
		return dt
	}
	return -dt
}

func TestTruncateTimestamp(t *testing.T) {
	t.Log("TestTruncateTimestamp")
	wd, clean := setupUfs(t)
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

func TestRemoveAll(t *testing.T) {
	t.Log("TestRemoveAll")
	wd, clean := setupUfs(t)
	defer clean()

	err := os.Mkdir(wd + "/ro/dir", 0755)
	CheckSuccess(err)
	
	contents := "hello"
	fn := wd + "/ro/dir/y"
	err = ioutil.WriteFile(fn, []byte(contents), 0644)
	CheckSuccess(err)

	err = os.RemoveAll(wd+"/mount/dir")
	if err != nil {
		t.Error("Should delete all")
	}
}

