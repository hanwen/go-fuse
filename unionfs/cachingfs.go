package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"log"
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
	origStream, code := fs.OpenDir(name, nil)

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
	a, code := fs.GetAttr(name, nil)
	return &attrResponse{
		FileInfo: a,
		Status:   code,
	}
}

func getXAttr(fs fuse.FileSystem, nameAttr string) *xattrResponse {
	ns := strings.SplitN(nameAttr, _XATTRSEP, 2)
	a, code := fs.GetXAttr(ns[0], ns[1], nil)
	return &xattrResponse{
		data:   a,
		Status: code,
	}
}

func readLink(fs fuse.FileSystem, name string) *linkResponse {
	a, code := fs.Readlink(name, nil)
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
	}, ttlNs)
	return c
}

func (me *CachingFileSystem) DropCache() {
	for _, c := range []*TimedCache{me.attributes, me.dirs, me.links, me.xattr} {
		c.DropAll(nil)
	}
}

func (me *CachingFileSystem) GetAttr(name string, context *fuse.Context) (*os.FileInfo, fuse.Status) {
	if name == _DROP_CACHE {
		return &os.FileInfo{
			Mode: fuse.S_IFREG | 0777,
		}, fuse.OK
	}

	r := me.attributes.Get(name).(*attrResponse)
	return r.FileInfo, r.Status
}

func (me *CachingFileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	key := name + _XATTRSEP + attr
	r := me.xattr.Get(key).(*xattrResponse)
	return r.data, r.Status
}

func (me *CachingFileSystem) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	r := me.links.Get(name).(*linkResponse)
	return r.linkContent, r.Status
}

func (me *CachingFileSystem) OpenDir(name string, context *fuse.Context) (stream chan fuse.DirEntry, status fuse.Status) {
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

func (me *CachingFileSystem) String() string {
	return fmt.Sprintf("CachingFileSystem(%v)", me.FileSystem)
}

func (me *CachingFileSystem) Open(name string, flags uint32, context *fuse.Context) (f fuse.File, status fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 && name == _DROP_CACHE {
		log.Println("Dropping cache for", me)
		me.DropCache()
	}
	return me.FileSystem.Open(name, flags, context)
}
