package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
)
var _ = fmt.Println
type attrResponse struct {
	*fuse.Attr
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

// Caches readdir and getattr()
type CachingFileSystem struct {
	fuse.FileSystem

	attributes *TimedCache
	dirs       *TimedCache
	links      *TimedCache
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
		Attr:   a,
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
	return c
}

func (me *CachingFileSystem) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	r := me.attributes.Get(name).(*attrResponse)
	return r.Attr, r.Status
}

func (me *CachingFileSystem) Readlink(name string) (string, fuse.Status) {
	r := me.attributes.Get(name).(*linkResponse)
	return r.linkContent, r.Status
}

func (me *CachingFileSystem) OpenDir(name string) (stream chan fuse.DirEntry, status fuse.Status) {
	r := me.dirs.Get(name).(*dirResponse)
	if r.Status == fuse.OK {
		stream = make(chan fuse.DirEntry, len(r.entries))
		for _, d := range r.entries {
			stream <- d
		}
		close(stream)
		return stream, r.Status
	}

	return nil, r.Status
}
