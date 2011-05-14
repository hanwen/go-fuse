package unionfs

import (
	"crypto/md5"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"syscall"
	"path"
	"path/filepath"
	"sync"
	"strings"
	"time"
)

// TODO(hanwen): is md5 sufficiently fast?
func filePathHash(path string) string {
	dir, base := filepath.Split(path)

	h := md5.New()
	h.Write([]byte(dir))

	// TODO(hanwen): should use a tighter format, so we can pack
	// more results in a readdir() roundtrip.
	return fmt.Sprintf("%x-%s", h.Sum()[:8], base)
}

/*

 UnionFs implements a user-space union file system, which is
 stateless but efficient even if the writable branch is on NFS.


 Assumptions:

 * It uses a list of branches, the first of which (index 0) is thought
 to be writable, and the rest read-only.

 * It assumes that the number of deleted files is small relative to
 the total tree size.


 Implementation notes.

 * It piggybacks on the existing LoopbackFileSystem in Go-FUSE, so we
 don't have to translate back and forth between Go's and FUSE's API.

 * Deleting a file will put a file named
 /DELETIONS/HASH-OF-FULL-FILENAME into the writable overlay,
 containing the full filename itself.

 This is optimized for NFS usage: we want to minimize the number of
 NFS operations, which are slow.  By putting all whiteouts in one
 place, we can cheaply fetch the list of all deleted files.  Even
 without caching on our side, the kernel's negative dentry cache can
 answer is-deleted queries quickly.

*/
type UnionFs struct {
	fuse.DefaultFileSystem

	name string

	// The same, but as interfaces.
	fileSystems []fuse.FileSystem

	cachingFileSystems []*CachingFileSystem

	// A file-existence cache.
	deletionCache *DirCache

	// A file -> branch cache.
	branchCache *TimedCache

	options *UnionFsOptions
}

type UnionFsOptions struct {
	BranchCacheTTLSecs   float64
	DeletionCacheTTLSecs float64
	DeletionDirName      string
}

const (
	_DROP_CACHE = ".drop_cache"
)

func NewUnionFs(name string, fileSystems []fuse.FileSystem, options UnionFsOptions) *UnionFs {
	g := new(UnionFs)
	g.name = name
	g.options = &options
	for i, fs := range fileSystems {
		if i > 0 {
			cfs := NewCachingFileSystem(fs, 0)
			g.cachingFileSystems = append(g.cachingFileSystems, cfs)
			fs = cfs
		}

		g.fileSystems = append(g.fileSystems, fs)
	}

	writable := g.fileSystems[0]
	fi, code := writable.GetAttr(options.DeletionDirName)
	if code == fuse.ENOENT {
		code = writable.Mkdir(options.DeletionDirName, 0755)
		fi, code = writable.GetAttr(options.DeletionDirName)
	}
	if !code.Ok() || !fi.IsDirectory() {
		panic(fmt.Sprintf("could not create deletion path %v: %v",
			options.DeletionDirName, code))
	}

	g.deletionCache = NewDirCache(writable, options.DeletionDirName, int64(options.DeletionCacheTTLSecs*1e9))
	g.branchCache = NewTimedCache(
		func(n string) interface{} { return g.getBranchAttrNoCache(n) },
		int64(options.BranchCacheTTLSecs*1e9))
	g.branchCache.RecurringPurge()
	return g
}

////////////////
// Deal with all the caches.

func (me *UnionFs) isDeleted(name string) bool {
	marker := me.deletionPath(name)
	haveCache, found := me.deletionCache.HasEntry(filepath.Base(marker))
	if haveCache {
		return found
	}

	_, code := me.fileSystems[0].GetAttr(marker)

	if code == fuse.OK {
		return true
	}
	if code == fuse.ENOENT {
		return false
	}

	panic(fmt.Sprintf("Unexpected GetAttr return code %v %v", code, marker))
	return false
}

func (me *UnionFs) getBranch(name string) branchResult {
	name = stripSlash(name)
	r := me.branchCache.Get(name)
	return r.(branchResult)
}

type branchResult struct {
	attr   *os.FileInfo
	code   fuse.Status
	branch int
}

func (me *UnionFs) getBranchAttrNoCache(name string) branchResult {
	name = stripSlash(name)

	parent, base := path.Split(name)
	parent = stripSlash(parent)

	parentBranch := 0
	if base != "" {
		parentBranch = me.getBranch(parent).branch
	}
	for i, fs := range me.fileSystems {
		if i < parentBranch {
			continue
		}

		a, s := fs.GetAttr(name)
		if s.Ok() {
			// Make all files appear writable
			a.Mode |= 0222
			return branchResult{
				attr:   a,
				code:   s,
				branch: i,
			}
		} else {
			if s != fuse.ENOENT {
				log.Printf("getattr: %v:  Got error %v from branch %v", name, s, i)
			}
		}
	}
	return branchResult{nil, fuse.ENOENT, -1}
}

////////////////
// Deletion.

func (me *UnionFs) deletionPath(name string) string {
	return filepath.Join(me.options.DeletionDirName, filePathHash(name))
}

func (me *UnionFs) removeDeletion(name string) {
	marker := me.deletionPath(name)
	me.deletionCache.RemoveEntry(path.Base(marker))

	// os.Remove tries to be smart and issues a Remove() and
	// Rmdir() sequentially.  We want to skip the 2nd system call,
	// so use syscall.Unlink() directly.

	code := me.fileSystems[0].Unlink(marker)
	if !code.Ok() && code != fuse.ENOENT {
		log.Printf("error unlinking %s: %v", marker, code)
	}
}

func (me *UnionFs) putDeletion(name string) fuse.Status {
	marker := me.deletionPath(name)
	me.deletionCache.AddEntry(path.Base(marker))

	// Is there a WriteStringToFileOrDie ?
	writable := me.fileSystems[0]
	f, code := writable.Open(marker, uint32(os.O_TRUNC|os.O_WRONLY|os.O_CREATE))
	if !code.Ok() {
		log.Printf("could not create deletion file %v: %v",
			marker, code)
		return fuse.EPERM
	}
	defer f.Release()
	defer f.Flush()
	n, code := f.Write(&fuse.WriteIn{}, []byte(name))
	if int(n) != len(name) || !code.Ok() {
		panic(fmt.Sprintf("Error for writing %v: %v, %v (exp %v) %v", name, marker, n, len(name), code))
	}

	return fuse.OK
}

////////////////
// Promotion.

func (me *UnionFs) Promote(name string, srcResult branchResult) fuse.Status {
	writable := me.fileSystems[0]
	sourceFs := me.fileSystems[srcResult.branch]

	// Promote directories.
	me.promoteDirsTo(name)

	code := fuse.CopyFile(sourceFs, writable, name, name)
	if !code.Ok() {
		me.branchCache.GetFresh(name)
		return code
	} else {
		r := me.getBranch(name)
		r.branch = 0
		me.branchCache.Set(name, r)
	}

	return fuse.OK
}

////////////////////////////////////////////////////////////////
// Below: implement interface for a FileSystem.

func (me *UnionFs) Rmdir(path string) (code fuse.Status) {
	r := me.getBranch(path)
	if r.code != fuse.OK {
		return r.code
	}
	if !r.attr.IsDirectory() {
		return syscall.ENOTDIR
	}
	if r.branch > 0 {
		stream, code := me.fileSystems[r.branch].OpenDir(path)
		if code.Ok() {
			_, ok := <-stream
			if ok {
				// TODO - should consume stream.
				return syscall.ENOTEMPTY
			}
		}
		me.putDeletion(path)
		return fuse.OK
	}

	code = me.fileSystems[0].Rmdir(path)
	if code != fuse.OK {
		return code
	}

	r = me.branchCache.GetFresh(path).(branchResult)
	if r.branch > 0 {
		code = me.putDeletion(path)
	}
	return code
}

func (me *UnionFs) Mkdir(path string, mode uint32) (code fuse.Status) {
	r := me.getBranch(path)
	if r.code != fuse.ENOENT {
		return syscall.EEXIST
	}

	code = me.promoteDirsTo(path)
	if code.Ok() {
		code = me.fileSystems[0].Mkdir(path, mode)
	}
	if code.Ok() {
		me.removeDeletion(path)
		attr := &os.FileInfo{
			Mode: fuse.S_IFDIR | mode | 0222,
		}
		me.branchCache.Set(path, branchResult{attr, fuse.OK, 0})
	}
	return code
}

func (me *UnionFs) Symlink(pointedTo string, linkName string) (code fuse.Status) {
	code = me.fileSystems[0].Symlink(pointedTo, linkName)
	if code.Ok() {
		me.removeDeletion(linkName)
		me.branchCache.GetFresh(linkName)
	}
	return code
}

func (me *UnionFs) Truncate(path string, offset uint64) (code fuse.Status) {
	r := me.getBranch(path)
	if r.branch > 0 {
		code = me.Promote(path, r)
		r.branch = 0
	}

	if code.Ok() {
		code = me.fileSystems[0].Truncate(path, offset)
	}
	if code.Ok() {
		r.attr.Size = int64(offset)
		now := time.Nanoseconds()
		r.attr.Mtime_ns = now
		r.attr.Ctime_ns = now
		me.branchCache.Set(path, r)
	}
	return code
}

func (me *UnionFs) Utimens(name string, atime uint64, mtime uint64) (code fuse.Status) {
	name = stripSlash(name)
	r := me.getBranch(name)

	code = r.code
	if code.Ok() && r.branch > 0 {
		code = me.Promote(name, r)
		r.branch = 0
	}
	if code.Ok() {
		code = me.fileSystems[0].Utimens(name, atime, mtime)
	}
	if code.Ok() {
		r.attr.Atime_ns = int64(atime)
		r.attr.Mtime_ns = int64(mtime)
		r.attr.Ctime_ns = time.Nanoseconds()
		me.branchCache.Set(name, r)
	}
	return code
}

func (me *UnionFs) Chown(name string, uid uint32, gid uint32) (code fuse.Status) {
	name = stripSlash(name)
	r := me.getBranch(name)
	if r.attr == nil || r.code != fuse.OK {
		return r.code
	}

	if os.Geteuid() != 0 {
		return fuse.EPERM
	}

	if r.attr.Uid != int(uid) || r.attr.Gid != int(gid) {
		if r.branch > 0 {
			code := me.Promote(name, r)
			if code != fuse.OK {
				return code
			}
			r.branch = 0
		}
		me.fileSystems[0].Chown(name, uid, gid)
	}
	r.attr.Uid = int(uid)
	r.attr.Gid = int(gid)
	r.attr.Ctime_ns = time.Nanoseconds()
	me.branchCache.Set(name, r)
	return fuse.OK
}

func (me *UnionFs) Chmod(name string, mode uint32) (code fuse.Status) {
	name = stripSlash(name)
	r := me.getBranch(name)
	if r.attr == nil {
		return r.code
	}
	if r.code != fuse.OK {
		return r.code
	}

	permMask := uint32(07777)

	// Always be writable.
	mode |= 0222
	oldMode := r.attr.Mode & permMask

	if oldMode != mode {
		if r.branch > 0 {
			code := me.Promote(name, r)
			if code != fuse.OK {
				return code
			}
			r.branch = 0
		}
		me.fileSystems[0].Chmod(name, mode)
	}
	r.attr.Mode = (r.attr.Mode &^ permMask) | mode
	r.attr.Ctime_ns = time.Nanoseconds()
	me.branchCache.Set(name, r)
	return fuse.OK
}

func (me *UnionFs) Access(name string, mode uint32) (code fuse.Status) {
	r := me.getBranch(name)
	if r.branch >= 0 {
		return me.fileSystems[r.branch].Access(name, mode)
	}

	return fuse.ENOENT
}

func (me *UnionFs) Unlink(name string) (code fuse.Status) {
	r := me.getBranch(name)
	if r.branch == 0 {
		code = me.fileSystems[0].Unlink(name)
		if code != fuse.OK {
			return code
		}
		r = me.branchCache.GetFresh(name).(branchResult)
	}

	if r.branch > 0 {
		// It would be nice to do the putDeletion async.
		code = me.putDeletion(name)
	}
	return code
}

func (me *UnionFs) Readlink(name string) (out string, code fuse.Status) {
	r := me.getBranch(name)
	if r.branch >= 0 {
		return me.fileSystems[r.branch].Readlink(name)
	}
	return "", fuse.ENOENT
}

func IsDir(fs fuse.FileSystem, name string) bool {
	a, code := fs.GetAttr(name)
	return code.Ok() && a.IsDirectory()
}

func stripSlash(fn string) string {
	return strings.TrimRight(fn, string(filepath.Separator))
}

func (me *UnionFs) promoteDirsTo(filename string) fuse.Status {
	dirName, _ := filepath.Split(filename)
	dirName = stripSlash(dirName)

	var todo []string
	var results []branchResult
	for dirName != "" {
		r := me.getBranch(dirName)

		if r.code != fuse.OK {
			log.Println("path component does not exist", filename, dirName)
		}
		if !r.attr.IsDirectory() {
			log.Println("path component is not a directory.", dirName, r)
			return fuse.EPERM
		}
		if r.branch == 0 {
			break
		}
		todo = append(todo, dirName)
		results = append(results, r)
		dirName, _ = filepath.Split(dirName)
		dirName = stripSlash(dirName)
	}

	for i, _ := range todo {
		j := len(todo) - i - 1
		d := todo[j]
		log.Println("Promoting directory", d)
		code := me.fileSystems[0].Mkdir(d, 0755)
		if code != fuse.OK {
			log.Println("Error creating dir leading to path", d, code)
			return fuse.EPERM
		}
		r := results[j]
		r.branch = 0
		me.branchCache.Set(d, r)
	}
	return fuse.OK
}

func (me *UnionFs) Create(name string, flags uint32, mode uint32) (fuseFile fuse.File, code fuse.Status) {
	writable := me.fileSystems[0]

	code = me.promoteDirsTo(name)
	if code != fuse.OK {
		return nil, code
	}
	fuseFile, code = writable.Create(name, flags, mode)
	if code.Ok() {
		me.removeDeletion(name)

		now := time.Nanoseconds()
		a := os.FileInfo{
			Mode:     fuse.S_IFREG | mode | 0222,
			Ctime_ns: now,
			Mtime_ns: now,
		}
		me.branchCache.Set(name, branchResult{&a, fuse.OK, 0})
	}
	return fuseFile, code
}

func (me *UnionFs) GetAttr(name string) (a *os.FileInfo, s fuse.Status) {
	if name == _READONLY {
		return nil, fuse.ENOENT
	}
	if name == _DROP_CACHE {
		return &os.FileInfo{
			Mode: fuse.S_IFREG | 0777,
		},fuse.OK
	}
	if name == me.options.DeletionDirName {
		return nil, fuse.ENOENT
	}
	if me.isDeleted(name) {
		return nil, fuse.ENOENT
	}
	r := me.getBranch(name)
	if r.branch < 0 {
		return nil, fuse.ENOENT
	}
	return r.attr, r.code
}

func (me *UnionFs) GetXAttr(name string, attr string) ([]byte, fuse.Status) {
	r := me.getBranch(name)
	if r.branch >= 0 {
		return me.fileSystems[r.branch].GetXAttr(name, attr)
	}
	return nil, fuse.ENOENT
}

func (me *UnionFs) OpenDir(directory string) (stream chan fuse.DirEntry, status fuse.Status) {
	dirBranch := me.getBranch(directory)
	if dirBranch.branch < 0 {
		return nil, fuse.ENOENT
	}

	// We could try to use the cache, but we have a delay, so
	// might as well get the fresh results async.
	var wg sync.WaitGroup
	var deletions map[string]bool

	wg.Add(1)
	go func() {
		deletions = newDirnameMap(me.fileSystems[0], me.options.DeletionDirName)
		wg.Done()
	}()

	entries := make([]map[string]uint32, len(me.fileSystems))
	for i, _ := range me.fileSystems {
		entries[i] = make(map[string]uint32)
	}

	statuses := make([]fuse.Status, len(me.fileSystems))
	for i, l := range me.fileSystems {
		if i >= dirBranch.branch {
			wg.Add(1)
			go func(j int, pfs fuse.FileSystem) {
				ch, s := pfs.OpenDir(directory)
				statuses[j] = s
				for s.Ok() {
					v, ok := <-ch
					if !ok {
						break
					}
					entries[j][v.Name] = v.Mode
				}
				wg.Done()
			}(i, l)
		}
	}

	wg.Wait()

	results := entries[0]

	// TODO(hanwen): should we do anything with the return
	// statuses?
	for i, m := range entries {
		if statuses[i] != fuse.OK {
			continue
		}
		if i == 0 {
			// We don't need to further process the first
			// branch: it has no deleted files.
			continue
		}
		for k, v := range m {
			_, ok := results[k]
			if ok {
				continue
			}

			deleted := deletions[filePathHash(filepath.Join(directory, k))]
			if !deleted {
				results[k] = v
			}
		}
	}
	if directory == "" {
		results[me.options.DeletionDirName] = 0, false
		// HACK.
		results[_READONLY] = 0, false
	}

	stream = make(chan fuse.DirEntry, len(results))
	for k, v := range results {
		stream <- fuse.DirEntry{
			Name: k,
			Mode: v,
		}
	}
	close(stream)
	return stream, fuse.OK
}

func (me *UnionFs) Rename(src string, dst string) (code fuse.Status) {
	srcResult := me.getBranch(src)
	code = srcResult.code
	if code.Ok() {
		code = srcResult.code
	}
	if code.Ok() && srcResult.branch > 0 {
		code = me.Promote(src, srcResult)
	}
	if code.Ok() {
		code = me.promoteDirsTo(dst)
	}
	if code.Ok() {
		code = me.fileSystems[0].Rename(src, dst)
	}

	if code.Ok() {
		me.removeDeletion(dst)
		srcResult.branch = 0
		me.branchCache.Set(dst, srcResult)

		if srcResult.branch == 0 {
			srcResult := me.branchCache.GetFresh(src)
			if srcResult.(branchResult).branch > 0 {
				code = me.putDeletion(src)
			}
		} else {
			code = me.putDeletion(src)
		}
	}
	return code
}

func (me *UnionFs) DropCaches() {
	log.Println("Forced cache drop on", me.name)
	me.branchCache.DropAll()
	me.deletionCache.DropCache()
	for _, fs := range me.cachingFileSystems {
		fs.DropCache()
	}
}

func (me *UnionFs) Open(name string, flags uint32) (fuseFile fuse.File, status fuse.Status) {
	if name == _DROP_CACHE {
		if flags&fuse.O_ANYWRITE != 0 {
			me.DropCaches()
		}
		return fuse.NewDevNullFile(), fuse.OK
	}
	r := me.getBranch(name)
	if flags&fuse.O_ANYWRITE != 0 && r.branch > 0 {
		code := me.Promote(name, r)
		if code != fuse.OK {
			return nil, code
		}
		r.branch = 0
		r.attr.Mtime_ns = time.Nanoseconds()
		me.branchCache.Set(name, r)
	}
	return me.fileSystems[r.branch].Open(name, uint32(flags))
}

func (me *UnionFs) Flush(name string) fuse.Status {
	// Refresh timestamps and size field.
	me.branchCache.GetFresh(name)
	return fuse.OK
}

func (me *UnionFs) Name() string {
	return me.name
}
