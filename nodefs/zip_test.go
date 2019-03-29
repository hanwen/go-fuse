// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"archive/zip"
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
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

	root := &zipRoot{zr: r}
	mntDir, clean := testMount(t, root, nil)
	defer clean()

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

func TestZipFSOnAdd(t *testing.T) {
	zipBytes := createZip(testData)

	r, err := zip.NewReader(&byteReaderAt{zipBytes}, int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}

	zr := &zipRoot{zr: r}

	root := &Inode{}
	mnt, clean := testMount(t, root, &Options{
		OnAdd: func(ctx context.Context) {
			root.AddChild("sub",
				root.NewPersistentInode(ctx, zr, NodeAttr{Mode: syscall.S_IFDIR}), false)
		},
	})
	defer clean()
	c, err := ioutil.ReadFile(mnt + "/sub/dir/subdir/subfile")
	if err != nil {
		t.Fatal("ReadFile", err)
	}
	if got, want := string(c), "content3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// zipFile is a file read from a zip archive.
type zipFile struct {
	Inode
	file *zip.File

	mu   sync.Mutex
	data []byte
}

var _ = (Opener)((*zipFile)(nil))
var _ = (Getattrer)((*zipFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
func (zf *zipFile) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = zf.file.UncompressedSize64
	return OK
}

// Open lazily unpacks zip data
func (zf *zipFile) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	zf.mu.Lock()
	defer zf.mu.Unlock()
	if zf.data == nil {
		rc, err := zf.file.Open()
		if err != nil {
			return nil, 0, syscall.EIO
		}
		content, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, 0, syscall.EIO
		}

		zf.data = content
	}

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, OK
}

// Read simply returns the data that was already unpacked in the Open call
func (zf *zipFile) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(zf.data) {
		end = len(zf.data)
	}
	return fuse.ReadResultData(zf.data[off:end]), OK
}

// zipRoot is the root of the Zip filesystem. Its only functionality
// is populating the filesystem.
type zipRoot struct {
	Inode

	zr *zip.Reader
}

var _ = (OnAdder)((*zipRoot)(nil))

func (zr *zipRoot) OnAdd(ctx context.Context) {
	// OnAdd is called once we are attached to an Inode. We can
	// then construct a tree.  We construct the entire tree, and
	// we don't want parts of the tree to disappear when the
	// kernel is short on memory, so we use persistent inodes.
	for _, f := range zr.zr.File {
		dir, base := filepath.Split(f.Name)

		p := &zr.Inode
		for _, component := range strings.Split(dir, "/") {
			if len(component) == 0 {
				continue
			}
			ch := p.GetChild(component)
			if ch == nil {
				ch = p.NewPersistentInode(ctx, &Inode{},
					NodeAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}
		ch := p.NewPersistentInode(ctx, &zipFile{file: f}, NodeAttr{})
		p.AddChild(base, ch, true)
	}
}

// Persistent inodes can be used to create an in-memory
// prefabricated file system tree.
func ExampleInode_NewPersistentInode() {
	// This is where we'll mount the FS
	mntDir, _ := ioutil.TempDir("", "")

	files := map[string]string{
		"file":              "content",
		"subdir/other-file": "other-content",
	}

	root := &Inode{}
	populate := func(ctx context.Context) {
		for name, content := range files {
			dir, base := filepath.Split(name)

			p := root

			// Add directories leading up to the file.
			for _, component := range strings.Split(dir, "/") {
				if len(component) == 0 {
					continue
				}
				ch := p.GetChild(component)
				if ch == nil {
					// Create a directory
					ch = p.NewPersistentInode(ctx, &Inode{},
						NodeAttr{Mode: syscall.S_IFDIR})
					// Add it
					p.AddChild(component, ch, true)
				}

				p = ch
			}

			// Create the file
			child := p.NewPersistentInode(ctx, &MemRegularFile{
				Data: []byte(content),
			}, NodeAttr{})

			// And add it
			p.AddChild(base, child, true)
		}
	}
	server, err := Mount(mntDir, root, &Options{
		MountOptions: fuse.MountOptions{Debug: true},

		// This adds read permissions to the files and
		// directories, which is necessary for doing a chdir
		// into the mount.
		DefaultPermissions: true,
		OnAdd:              populate,
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted on %s", mntDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", mntDir)
	server.Wait()
}
