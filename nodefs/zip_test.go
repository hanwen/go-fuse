// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"archive/zip"
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

var testData = map[string]string{
	"file.txt":           "content",
	"dir/subfile1":       "content2",
	"dir/subdir/subfile": "content3",
}

func createZip(data map[string]string) []byte {
	buf := &bytes.Buffer{}

	zw := zip.NewWriter(buf)
	for k, v := range data {
		fw, _ := zw.Create(k)
		fw.Write([]byte(v))
	}

	zw.Close()
	return buf.Bytes()
}

type byteReaderAt struct {
	b []byte
}

func (br *byteReaderAt) ReadAt(data []byte, off int64) (int, error) {
	end := int(off) + len(data)
	if end > len(br.b) {
		end = len(br.b)
	}

	copy(data, br.b[off:end])
	return end - int(off), nil
}

func TestZipFS(t *testing.T) {
	zipBytes := createZip(testData)

	r, err := zip.NewReader(&byteReaderAt{zipBytes}, int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}

	root := &zipRoot{r: r}

	mntDir := testutil.TempDir()

	rawFS := NewNodeFS(root, &Options{
		Debug:             testutil.VerboseTest(),
		FirstAutomaticIno: 1,
	})
	server, err := fuse.NewServer(rawFS, mntDir,
		&fuse.MountOptions{
			Debug: testutil.VerboseTest(),
		})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}

	for k, v := range testData {
		c, err := ioutil.ReadFile(filepath.Join(mntDir, k))
		if err != nil {
			t.Fatal(err)
		}
		if string(c) != v {
			t.Errorf("got %q, want %q", c, v)
		}
	}

	entries, err := ioutil.ReadDir(mntDir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = e.IsDir()
	}

	want := map[string]bool{
		"dir": true, "file.txt": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// zipFile is a file read from a zip file.
type zipFile struct {
	DefaultOperations
	file *zip.File

	mu   sync.Mutex
	data []byte
}

var _ = (FileOperations)((*zipFile)(nil))

// GetAttr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
func (zf *zipFile) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
	out.Size = zf.file.UncompressedSize64
	return fuse.OK
}

// Open lazily unpacks zip data
func (zf *zipFile) Open(ctx context.Context, flags uint32) (FileHandle, uint32, fuse.Status) {
	zf.mu.Lock()
	defer zf.mu.Unlock()
	if zf.data == nil {
		rc, err := zf.file.Open()
		if err != nil {
			return nil, 0, fuse.EIO
		}
		content, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, 0, fuse.EIO
		}

		zf.data = content
	}

	// We don't return a filehandle since we don't really need
	// one.
	return nil, 0, fuse.OK
}

// Read  sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
func (zf *zipFile) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	end := int(off) + len(dest)
	if end > len(zf.data) {
		end = len(zf.data)
	}
	return fuse.ReadResultData(zf.data[off:end]), fuse.OK
}

// zipRoot is the root of the Zip filesystem. Its only functionality
// is populating the filesystem.
type zipRoot struct {
	DefaultOperations

	r *zip.Reader
}

func (zr *zipRoot) OnAdd() {
	// OnAdd is called once we are attached to an Inode. We can
	// then construct a tree.  We construct the entire tree, and
	// we don't want parts of the tree to disappear when the
	// kernel is short on memory, so we use persistent inodes.
	for _, f := range zr.r.File {
		dir, base := filepath.Split(f.Name)

		p := InodeOf(zr)
		for _, component := range strings.Split(dir, "/") {
			if len(component) == 0 {
				continue
			}
			ch := p.GetChild(component)
			if ch == nil {
				ch = InodeOf(zr).NewPersistentInode(&DefaultOperations{},
					NodeAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}
		ch := InodeOf(zr).NewPersistentInode(&zipFile{file: f}, NodeAttr{Mode: fuse.S_IFREG})
		p.AddChild(base, ch, true)
	}
}
