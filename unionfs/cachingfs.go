package unionfs

import (
	"bytes"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"strings"
)

var _ = fmt.Println

const _XATTRSEP = "@XATTR@"

type attrResponse struct {
	*os.FileInfo
	fuse.Status
}

type xattrResponse struct {
	data []byte
	fuse.Status
}

type dirResponse struct {
	entries []fuse.DirEntry
	fuse.Status
}

type linkResponse struct {
	linkContent string
	fuse.Status
}

type openResponse struct {
	fuse.File
	fuse.Status
}

// Caches filesystem metadata.
type CachingFileSystem struct {
	fuse.FileSystem

	attributes *TimedCache
	dirs       *TimedCache
	links      *TimedCache
	xattr      *TimedCache
	files      *TimedCache
}

func readDir(fs fuse.FileSystem, name string) *dirResponse {
	origStream, code := fs.OpenDir(name)

	r := &dirResponse{nil, code}
	if code != fuse.OK {
		return r
	}

	for {
		d, ok := <-origStream
		if !ok {
			break
		}
		r.entries = append(r.entries, d)
	}
	return r
}

func getAttr(fs fuse.FileSystem, name string) *attrResponse {
	a, code := fs.GetAttr(name)
	return &attrResponse{
		FileInfo: a,
		Status:   code,
	}
}

func openFile(fs fuse.FileSystem, name string) (result *openResponse) {
	result = &openResponse{}
	flags := uint32(os.O_RDONLY)

	f, code := fs.Open(name, flags)
	if !code.Ok() {
		result.Status = code
		return
	}
	defer f.Release()
	defer f.Flush()

	buf := bytes.NewBuffer(nil)
	input := fuse.ReadIn{
		Offset: 0,
		Size:   128 * (1 << 10),
		Flags:  flags,
	}

	bp := fuse.NewGcBufferPool()
	for {
		data, status := f.Read(&input, bp)
		buf.Write(data)
		if !status.Ok() {
			result.Status = status
			return
		}
		if len(data) < int(input.Size) {
			break
		}
		input.Offset += uint64(len(data))
	}

	result.File = fuse.NewReadOnlyFile(buf.Bytes())
	return
}

func getXAttr(fs fuse.FileSystem, nameAttr string) *xattrResponse {
	ns := strings.Split(nameAttr, _XATTRSEP, 2)
	a, code := fs.GetXAttr(ns[0], ns[1])
	return &xattrResponse{
		data:   a,
		Status: code,
	}
}

func readLink(fs fuse.FileSystem, name string) *linkResponse {
	a, code := fs.Readlink(name)
	return &linkResponse{
		linkContent: a,
		Status:      code,
	}
}

func NewCachingFileSystem(fs fuse.FileSystem, ttlNs int64) *CachingFileSystem {
	c := new(CachingFileSystem)
	c.FileSystem = fs
	c.attributes = NewTimedCache(func(n string) interface{} { return getAttr(fs, n) }, ttlNs)
	c.dirs = NewTimedCache(func(n string) interface{} { return readDir(fs, n) }, ttlNs)
	c.links = NewTimedCache(func(n string) interface{} { return readLink(fs, n) }, ttlNs)
	c.xattr = NewTimedCache(func(n string) interface{} {
		return getXAttr(fs, n)
	},ttlNs)
	c.files = NewTimedCache(func(n string) interface{} {
		return openFile(fs, n)
	},ttlNs)
	return c
}

func (me *CachingFileSystem) DropCache() {
	for _, c := range []*TimedCache{me.attributes, me.dirs, me.links, me.xattr} {
		c.DropAll()
	}
}

func (me *CachingFileSystem) GetAttr(name string) (*os.FileInfo, fuse.Status) {
	r := me.attributes.Get(name).(*attrResponse)
	return r.FileInfo, r.Status
}

func (me *CachingFileSystem) GetXAttr(name string, attr string) ([]byte, fuse.Status) {
	key := name + _XATTRSEP + attr
	r := me.xattr.Get(key).(*xattrResponse)
	return r.data, r.Status
}

func (me *CachingFileSystem) Readlink(name string) (string, fuse.Status) {
	r := me.links.Get(name).(*linkResponse)
	return r.linkContent, r.Status
}

func (me *CachingFileSystem) OpenDir(name string) (stream chan fuse.DirEntry, status fuse.Status) {
	r := me.dirs.Get(name).(*dirResponse)
	if r.Status.Ok() {
		stream = make(chan fuse.DirEntry, len(r.entries))
		for _, d := range r.entries {
			stream <- d
		}
		close(stream)
		return stream, r.Status
	}

	return nil, r.Status
}

// Caching file contents easily overflows available memory.
func (me *CachingFileSystem) DisabledOpen(name string, flags uint32) (f fuse.File, status fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	r := me.files.Get(name).(*openResponse)
	return r.File, r.Status
}
