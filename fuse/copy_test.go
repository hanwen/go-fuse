package fuse

import (
	"testing"
	"io/ioutil"
	"os"
)

func TestCopyFile(t *testing.T) {
	d1, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	defer os.RemoveAll(d1)
	d2, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	defer os.RemoveAll(d2)

	fs1 := NewLoopbackFileSystem(d1)
	fs2 := NewLoopbackFileSystem(d2)

	content1 := "blabla"

	err = ioutil.WriteFile(d1+"/file", []byte(content1), 0644)
	CheckSuccess(err)

	code := CopyFile(fs1, fs2, "file", "file", nil)
	if !code.Ok() {
		t.Fatal("Unexpected ret code", code)
	}

	data, err := ioutil.ReadFile(d2 + "/file")
	if content1 != string(data) {
		t.Fatal("Unexpected content", string(data))
	}

	content2 := "foobar"

	err = ioutil.WriteFile(d2+"/file", []byte(content2), 0644)
	CheckSuccess(err)

	// Copy back: should overwrite.
	code = CopyFile(fs2, fs1, "file", "file", nil)
	if !code.Ok() {
		t.Fatal("Unexpected ret code", code)
	}

	data, err = ioutil.ReadFile(d1 + "/file")
	if content2 != string(data) {
		t.Fatal("Unexpected content", string(data))
	}

}
