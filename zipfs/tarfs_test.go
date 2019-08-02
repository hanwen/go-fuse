// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zipfs

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

var tarContents = map[string]string{
	"emptydir/":       "",
	"file.txt":        "content",
	"dir/subfile.txt": "other content",
}

type addClose struct {
	io.Reader
}

func (c *addClose) Close() error {
	return nil
}

func TestTar(t *testing.T) {
	buf := &bytes.Buffer{}

	w := tar.NewWriter(buf)
	now := time.Now()
	for k, v := range tarContents {
		h := &tar.Header{
			Name:    k,
			Size:    int64(len(v)),
			Mode:    0464,
			Uid:     42,
			Gid:     42,
			ModTime: now,
		}

		isLink := filepath.Base(k) == "link"
		isDir := strings.HasSuffix(k, "/")
		if isLink {
			h.Typeflag = tar.TypeSymlink
			h.Linkname = v
		} else if isDir {
			h.Typeflag = tar.TypeDir
		}

		w.WriteHeader(h)
		if !isLink && !isDir {
			w.Write([]byte(v))
		}
	}
	w.Close()

	root := &tarRoot{rc: &addClose{buf}}

	mnt := testutil.TempDir()
	defer os.Remove(mnt)
	opts := &fs.Options{}
	opts.Debug = testutil.VerboseTest()
	s, err := fs.Mount(mnt, root, opts)
	if err != nil {
		t.Errorf("Mount: %v", err)
	}
	defer s.Unmount()

	for k, want := range tarContents {
		p := filepath.Join(mnt, k)
		var st syscall.Stat_t
		if err := syscall.Lstat(p, &st); err != nil {
			t.Fatalf("Stat %q: %v", p, err)
		}

		if filepath.Base(k) == "link" {
			got, err := os.Readlink(p)
			if err != nil {
				t.Fatalf("Readlink: %v", err)
			}

			if got != want {
				t.Errorf("Readlink: got %q want %q", got, want)
			}
		} else if strings.HasSuffix(k, "/") {

			if got, want := st.Mode, uint32(syscall.S_IFDIR|0464); got != want {
				t.Errorf("dir %q: got mode %o, want %o", k, got, want)
			}

		} else {
			if got, want := st.Mode, uint32(syscall.S_IFREG|0464); got != want {
				t.Errorf("entry %q, got mode %o, want %o", k, got, want)
			}

			c, err := ioutil.ReadFile(p)
			if err != nil {
				t.Errorf("read %q: %v", k, err)
				got := string(c)
				if got != want {
					t.Errorf("file %q: got %q, want %q", k, got, want)
				}
			}
		}
	}
}
