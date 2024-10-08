// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"os"
	"testing"
)

func TestCopyFile(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()

	fs1 := NewLoopbackFileSystem(d1)
	fs2 := NewLoopbackFileSystem(d2)

	content1 := "blabla"

	err := os.WriteFile(d1+"/file", []byte(content1), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	code := CopyFile(fs1, fs2, "file", "file", nil)
	if !code.Ok() {
		t.Fatal("Unexpected ret code", code)
	}

	data, err := os.ReadFile(d2 + "/file")
	if content1 != string(data) {
		t.Fatal("Unexpected content", string(data))
	}

	content2 := "foobar"

	err = os.WriteFile(d2+"/file", []byte(content2), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Copy back: should overwrite.
	code = CopyFile(fs2, fs1, "file", "file", nil)
	if !code.Ok() {
		t.Fatal("Unexpected ret code", code)
	}

	data, err = os.ReadFile(d1 + "/file")
	if content2 != string(data) {
		t.Fatal("Unexpected content", string(data))
	}

}
