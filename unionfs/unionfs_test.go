package unionfs

import (
	"os"
	"github.com/hanwen/go-fuse/fuse"
	"fmt"
	"log"
	"testing"
)

var _ = fmt.Print
var _ = log.Print
var CheckSuccess = fuse.CheckSuccess

func TestFilePathHash(t *testing.T) {
	// Simple test coverage.
	t.Log(filePathHash("xyz/abc"))
}


var testOpts = UnionFsOptions{
	DeletionCacheTTLSecs: 0.01,
	DeletionDirName:      "DELETIONS",
	BranchCacheTTLSecs:   0.01,
}

func setup(t *testing.T) (workdir string, state *fuse.MountState) {
	wd := fuse.MakeTempDir()
	err := os.Mkdir(wd+"/mount", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/rw", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)

	var roots []string
	roots = append(roots, wd+"/rw")
	roots = append(roots, wd+"/ro")
	ufs := NewUnionfs(roots, testOpts)

	connector := fuse.NewPathFileSystemConnector(ufs)
	state = fuse.NewMountState(connector)
	state.Mount(wd + "/mount")

	go state.Loop(false)

	return wd, state
}

func writeToFile(path string, contents string, create bool) {
	var flags int = os.O_WRONLY
	if create {
		flags |= os.O_CREATE
	}

	f, err := os.OpenFile(path, flags, 0644)
	fuse.CheckSuccess(err)

	_, err = f.Write([]byte(contents))
	fuse.CheckSuccess(err)

	err = f.Close()
	fuse.CheckSuccess(err)
}

func readFromFile(path string) string {
	f, err := os.Open(path)
	fuse.CheckSuccess(err)

	fi, err := os.Stat(path)
	content := make([]byte, fi.Size)
	n, err := f.Read(content)
	fuse.CheckSuccess(err)
	if n < int(fi.Size) {
		panic("short read.")
	}

	err = f.Close()
	fuse.CheckSuccess(err)
	return string(content)
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
		ok, val := m2[k]
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
	wd, state := setup(t)
	defer state.Unmount()

	err := os.Symlink("/foobar", wd+"/mount/link")
	CheckSuccess(err)

	val, err := os.Readlink(wd + "/mount/link")
	CheckSuccess(err)

	if val != "/foobar" {
		t.Errorf("symlink mismatch: %v", val)
	}
}

func TestBasic(t *testing.T) {
	wd, state := setup(t)
	defer state.Unmount()

	writeToFile(wd+"/rw/rw", "a", true)
	writeToFile(wd+"/ro/ro1", "a", true)
	writeToFile(wd+"/ro/ro2", "b", true)

	names := dirNames(wd + "/mount")
	expected := map[string]bool{
		"rw": true, "ro1": true, "ro2": true,
	}
	checkMapEq(t, names, expected)

	writeToFile(wd+"/mount/new", "new contents", true)
	if !fileExists(wd + "/rw/new") {
		t.Errorf("missing file in rw layer", names)
	}

	if readFromFile(wd+"/mount/new") != "new contents" {
		t.Errorf("read mismatch.")
	}

	writeToFile(wd+"/mount/ro1", "promote me", false)
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
