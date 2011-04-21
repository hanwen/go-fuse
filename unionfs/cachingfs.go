package unionfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"sync"
)

type attrResponse struct {
	attr *fuse.Attr
	code fuse.Status
}

type dirResponse struct {
	entries []fuse.DirEntry
	code    fuse.Status
}

type linkResponse struct {
	linkContent string
	code        fuse.Status
}

// Caches readdir and getattr()
type CachingFileSystem struct {
	fuse.WrappingPathFileSystem

	attributesLock sync.RWMutex
	attributes     map[string]attrResponse

	dirsLock sync.RWMutex
	dirs     map[string]dirResponse

	linksLock sync.RWMutex
	links     map[string]linkResponse
}

func NewCachingFileSystem(pfs fuse.PathFileSystem) *CachingFileSystem {
	c := new(CachingFileSystem)
	c.Original = pfs
	c.attributes = make(map[string]attrResponse)
	c.dirs = make(map[string]dirResponse)
	c.links = make(map[string]linkResponse)
	return c
}

func (me *CachingFileSystem) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	me.attributesLock.RLock()
	v, ok := me.attributes[name]
	me.attributesLock.RUnlock()

	if ok {
		return v.attr, v.code
	}

	var r attrResponse
	r.attr, r.code = me.Original.GetAttr(name)

	// TODO - could do async.
	me.attributesLock.Lock()
	me.attributes[name] = r
	me.attributesLock.Unlock()

	return r.attr, r.code
}

func (me *CachingFileSystem) Readlink(name string) (string, fuse.Status) {
	me.linksLock.RLock()
	v, ok := me.links[name]
	me.linksLock.RUnlock()

	if ok {
		return v.linkContent, v.code
	}

	v.linkContent, v.code = me.Original.Readlink(name)

	// TODO - could do async.
	me.linksLock.Lock()
	me.links[name] = v
	me.linksLock.Unlock()

	return v.linkContent, v.code
}

func (me *CachingFileSystem) OpenDir(name string) (stream chan fuse.DirEntry, status fuse.Status) {
	me.dirsLock.RLock()
	v, ok := me.dirs[name]
	me.dirsLock.RUnlock()

	if !ok {
		origStream, code := me.Original.OpenDir(name)
		if code != fuse.OK {
			return nil, code
		}

		v.code = code
		for {
			d := <-origStream
			if d.Name == "" {
				break
			}
			v.entries = append(v.entries, d)
		}

		me.dirsLock.Lock()
		me.dirs[name] = v
		me.dirsLock.Unlock()
	}

	stream = make(chan fuse.DirEntry)
	go func() {
		for _, d := range v.entries {
			stream <- d
		}
		stream <- fuse.DirEntry{}
	}()

	return stream, v.code
}
