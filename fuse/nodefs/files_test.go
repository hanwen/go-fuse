// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// Check that loopbackFile.Utimens() works as expected
func TestLoopbackFileUtimens(t *testing.T) {
	f2, err := ioutil.TempFile("", "TestLoopbackFileUtimens")
	if err != nil {
		t.Fatal(err)
	}
	path := f2.Name()
	defer os.Remove(path)
	defer f2.Close()
	f := NewLoopbackFile(f2)

	utimensFn := func(atime *time.Time, mtime *time.Time) fuse.Status {
		return f.Utimens(atime, mtime, &fuse.Context{Cancel: nil})
	}
	testutil.TestLoopbackUtimens(t, path, utimensFn)
}
