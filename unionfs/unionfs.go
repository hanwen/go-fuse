package unionfs

import (
	"crypto/md5"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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

 * It overlays arbitrary writable FileSystems with any number of
   readonly FileSystems.

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

	// The same, but as interfaces.
	fileSystems []fuse.FileSystem

	// A file-existence cache.
	deletionCache *DirCache

	// A file -> branch cache.
	branchCache *TimedCache

	options *UnionFsOptions
	nodeFs  *fuse.PathNodeFs
}

type UnionFsOptions struct {
	BranchCacheTTLSecs   float64
	DeletionCacheTTLSecs float64
	DeletionDirName      string
}

const (
	_DROP_CACHE = ".drop_cache"
)

func NewUnionFs(fileSystems []fuse.FileSystem, options UnionFsOptions) *UnionFs {
	g := new(UnionFs)
	g.options = &options
	g.fileSystems = fileSystems

	writable := g.fileSystems[0]
	code := g.createDeletionStore()
	if !code.Ok() {
		log.Printf("could not create deletion path %v: %v", options.DeletionDirName, code)
		return nil
	}

	g.deletionCache = NewDirCache(writable, options.DeletionDirName, int64(options.DeletionCacheTTLSecs*1e9))
	g.branchCache = NewTimedCache(
		func(n string) interface{} { return g.getBranchAttrNoCache(n) },
		int64(options.BranchCacheTTLSecs*1e9))
	g.branchCache.RecurringPurge()
	return g
}

func (me *UnionFs) OnMount(nodeFs *fuse.PathNodeFs) {
	me.nodeFs = nodeFs
}

////////////////
// Deal with all the caches.

// The isDeleted() method tells us if a path has a marker in the deletion store.
// It may return an error code if the store could not be accessed.
func (me *UnionFs) isDeleted(name string) (deleted bool, code fuse.Status) {
	marker := me.deletionPath(name)
	haveCache, found := me.deletionCache.HasEntry(filepath.Base(marker))
	if haveCache {
		return found, fuse.OK
	}

	_, code = me.fileSystems[0].GetAttr(marker, nil)

	if code == fuse.OK {
		return true, code
	}
	if code == fuse.ENOENT {
		return false, fuse.OK
	}

	log.Println("error accessing deletion marker:", marker)
	return false, syscall.EROFS
}

func (me *UnionFs) createDeletionStore() (code fuse.Status) {
	writable := me.fileSystems[0]
	fi, code := writable.GetAttr(me.options.DeletionDirName, nil)
	if code == fuse.ENOENT {
		code = writable.Mkdir(me.options.DeletionDirName, 0755, nil)
		if code.Ok() {
			fi, code = writable.GetAttr(me.options.DeletionDirName, nil)
		}
	}

	if !code.Ok() || !fi.IsDirectory() {
		code = syscall.EROFS
	}

	return code
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

func printFileInfo(me *os.FileInfo) string {
	return fmt.Sprintf(
		"{0%o S=%d L=%d %d:%d %d*%d %d:%d "+
			"A %.09f M %.09f C %.09f}",
		me.Mode, me.Size, me.Nlink, me.Uid, me.Gid, me.Blocks, me.Blksize, me.Rdev, me.Ino,
		float64(me.Atime_ns)*1e-9, float64(me.Mtime_ns)*1e-9, float64(me.Ctime_ns)*1e-9)
}

func (me branchResult) String() string {
	return fmt.Sprintf("{%s %v branch %d}", printFileInfo(me.attr), me.code, me.branch)
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

		a, s := fs.GetAttr(name, nil)
		if s.Ok() {
			if i > 0 {
				// Needed to make hardlinks work.
				a.Ino = 0
			}
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

	code := me.fileSystems[0].Unlink(marker, nil)
	if !code.Ok() && code != fuse.ENOENT {
		log.Printf("error unlinking %s: %v", marker, code)
	}
}

func (me *UnionFs) putDeletion(name string) (code fuse.Status) {
	code = me.createDeletionStore()
	if !code.Ok() {
		return code
	}

	marker := me.deletionPath(name)
	me.deletionCache.AddEntry(path.Base(marker))

	// Is there a WriteStringToFileOrDie ?
	writable := me.fileSystems[0]
	fi, code := writable.GetAttr(marker, nil)
	if code.Ok() && fi.Size == int64(len(name)) {
		return fuse.OK
	}

	var f fuse.File
	if code == fuse.ENOENT {
		f, code = writable.Create(marker, uint32(os.O_TRUNC|os.O_WRONLY), 0644, nil)
	} else {
		writable.Chmod(marker, 0644, nil)
		f, code = writable.Open(marker, uint32(os.O_TRUNC|os.O_WRONLY), nil)
	}
	if !code.Ok() {
		log.Printf("could not create deletion file %v: %v", marker, code)
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

func (me *UnionFs) Promote(name string, srcResult branchResult, context *fuse.Context) (code fuse.Status) {
	writable := me.fileSystems[0]
	sourceFs := me.fileSystems[srcResult.branch]

	// Promote directories.
	me.promoteDirsTo(name)

	if srcResult.attr.IsRegular() {
		code = fuse.CopyFile(sourceFs, writable, name, name, context)

		if code.Ok() {
			code = writable.Chmod(name, srcResult.attr.Mode&07777|0200, context)
		}
		if code.Ok() {
			code = writable.Utimens(name, uint64(srcResult.attr.Atime_ns),
				uint64(srcResult.attr.Mtime_ns), context)
		}

		files := me.nodeFs.AllFiles(name, 0)
		for _, fileWrapper := range files {
			if !code.Ok() {
				break
			}
			var uf *unionFsFile
			f := fileWrapper.File
			for f != nil {
				ok := false
				uf, ok = f.(*unionFsFile)
				if ok {
					break
				}
				f = f.InnerFile()
			}
			if uf == nil {
				panic("no unionFsFile found inside")
			}

			if uf.layer > 0 {
				uf.layer = 0
				f := uf.File
				uf.File, code = me.fileSystems[0].Open(name, fileWrapper.OpenFlags, context)
				f.Flush()
				f.Release()
			}
		}
	} else if srcResult.attr.IsSymlink() {
		link := ""
		link, code = sourceFs.Readlink(name, context)
		if !code.Ok() {
			log.Println("can't read link in source fs", name)
		} else {
			code = writable.Symlink(link, name, context)
		}
	} else if srcResult.attr.IsDirectory() {
		code = writable.Mkdir(name, srcResult.attr.Mode&07777|0200, context)
	} else {
		log.Println("Unknown file type:", srcResult.attr)
		return fuse.ENOSYS
	}

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

func (me *UnionFs) Link(orig string, newName string, context *fuse.Context) (code fuse.Status) {
	origResult := me.getBranch(orig)
	code = origResult.code
	if code.Ok() && origResult.branch > 0 {
		code = me.Promote(orig, origResult, context)
	}
	if code.Ok() && origResult.branch > 0 {
		// Hairy: for the link to be hooked up to the existing
		// inode, PathNodeFs must see a client inode for the
		// original.  We force a refresh of the attribute (so
		// the Ino is filled in.), and then force PathNodeFs
		// to see the Inode number.
		me.branchCache.GetFresh(orig)
		inode := me.nodeFs.Node(orig)
		inode.FsNode().GetAttr(nil, nil)
	}
	if code.Ok() {
		code = me.promoteDirsTo(newName)
	}
	if code.Ok() {
		code = me.fileSystems[0].Link(orig, newName, context)
	}
	if code.Ok() {
		me.removeDeletion(newName)
		me.branchCache.GetFresh(newName)
	}
	return code
}

func (me *UnionFs) Rmdir(path string, context *fuse.Context) (code fuse.Status) {
	r := me.getBranch(path)
	if r.code != fuse.OK {
		return r.code
	}
	if !r.attr.IsDirectory() {
		return syscall.ENOTDIR
	}

	stream, code := me.OpenDir(path, context)
	found := false
	for _ = range stream {
		found = true
	}
	if found {
		return syscall.ENOTEMPTY
	}

	if r.branch > 0 {
		code = me.putDeletion(path)
		return code
	}
	code = me.fileSystems[0].Rmdir(path, context)
	if code != fuse.OK {
		return code
	}

	r = me.branchCache.GetFresh(path).(branchResult)
	if r.branch > 0 {
		code = me.putDeletion(path)
	}
	return code
}

func (me *UnionFs) Mkdir(path string, mode uint32, context *fuse.Context) (code fuse.Status) {
	deleted, code := me.isDeleted(path)
	if !code.Ok() {
		return code
	}

	if !deleted {
		r := me.getBranch(path)
		if r.code != fuse.ENOENT {
			return syscall.EEXIST
		}
	}

	code = me.promoteDirsTo(path)
	if code.Ok() {
		code = me.fileSystems[0].Mkdir(path, mode, context)
	}
	if code.Ok() {
		me.removeDeletion(path)
		attr := &os.FileInfo{
			Mode: fuse.S_IFDIR | mode,
		}
		me.branchCache.Set(path, branchResult{attr, fuse.OK, 0})
	}

	var stream chan fuse.DirEntry
	stream, code = me.OpenDir(path, context)
	if code.Ok() {
		// This shouldn't happen, but let's be safe.
		for entry := range stream {
			me.putDeletion(filepath.Join(path, entry.Name))
		}
	}

	return code
}

func (me *UnionFs) Symlink(pointedTo string, linkName string, context *fuse.Context) (code fuse.Status) {
	code = me.promoteDirsTo(linkName)
	if code.Ok() {
		code = me.fileSystems[0].Symlink(pointedTo, linkName, context)
	}
	if code.Ok() {
		me.removeDeletion(linkName)
		me.branchCache.GetFresh(linkName)
	}
	return code
}

func (me *UnionFs) Truncate(path string, size uint64, context *fuse.Context) (code fuse.Status) {
	if path == _DROP_CACHE {
		return fuse.OK
	}

	r := me.getBranch(path)
	if r.branch > 0 {
		code = me.Promote(path, r, context)
		r.branch = 0
	}

	if code.Ok() {
		code = me.fileSystems[0].Truncate(path, size, context)
	}
	if code.Ok() {
		r.attr.Size = int64(size)
		now := time.Nanoseconds()
		r.attr.Mtime_ns = now
		r.attr.Ctime_ns = now
		me.branchCache.Set(path, r)
	}
	return code
}

func (me *UnionFs) Utimens(name string, atime uint64, mtime uint64, context *fuse.Context) (code fuse.Status) {
	name = stripSlash(name)
	r := me.getBranch(name)

	code = r.code
	if code.Ok() && r.branch > 0 {
		code = me.Promote(name, r, context)
		r.branch = 0
	}
	if code.Ok() {
		code = me.fileSystems[0].Utimens(name, atime, mtime, context)
	}
	if code.Ok() {
		r.attr.Atime_ns = int64(atime)
		r.attr.Mtime_ns = int64(mtime)
		r.attr.Ctime_ns = time.Nanoseconds()
		me.branchCache.Set(name, r)
	}
	return code
}

func (me *UnionFs) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
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
			code := me.Promote(name, r, context)
			if code != fuse.OK {
				return code
			}
			r.branch = 0
		}
		me.fileSystems[0].Chown(name, uid, gid, context)
	}
	r.attr.Uid = int(uid)
	r.attr.Gid = int(gid)
	r.attr.Ctime_ns = time.Nanoseconds()
	me.branchCache.Set(name, r)
	return fuse.OK
}

func (me *UnionFs) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
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
	oldMode := r.attr.Mode & permMask

	if oldMode != mode {
		if r.branch > 0 {
			code := me.Promote(name, r, context)
			if code != fuse.OK {
				return code
			}
			r.branch = 0
		}
		me.fileSystems[0].Chmod(name, mode, context)
	}
	r.attr.Mode = (r.attr.Mode &^ permMask) | mode
	r.attr.Ctime_ns = time.Nanoseconds()
	me.branchCache.Set(name, r)
	return fuse.OK
}

func (me *UnionFs) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	// We always allow writing.
	mode = mode &^ fuse.W_OK
	if name == "" {
		return fuse.OK
	}
	r := me.getBranch(name)
	if r.branch >= 0 {
		return me.fileSystems[r.branch].Access(name, mode, context)
	}
	return fuse.ENOENT
}

func (me *UnionFs) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	r := me.getBranch(name)
	if r.branch == 0 {
		code = me.fileSystems[0].Unlink(name, context)
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

func (me *UnionFs) Readlink(name string, context *fuse.Context) (out string, code fuse.Status) {
	r := me.getBranch(name)
	if r.branch >= 0 {
		return me.fileSystems[r.branch].Readlink(name, context)
	}
	return "", fuse.ENOENT
}

func IsDir(fs fuse.FileSystem, name string) bool {
	a, code := fs.GetAttr(name, nil)
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

		if !r.code.Ok() {
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
		r := results[j]
		code := me.fileSystems[0].Mkdir(d, r.attr.Mode&07777|0200, nil)
		if code != fuse.OK {
			log.Println("Error creating dir leading to path", d, code, me.fileSystems[0])
			return fuse.EPERM
		}

		me.fileSystems[0].Utimens(d, uint64(r.attr.Atime_ns), uint64(r.attr.Mtime_ns), nil)
		r.branch = 0
		me.branchCache.Set(d, r)
	}
	return fuse.OK
}

func (me *UnionFs) Create(name string, flags uint32, mode uint32, context *fuse.Context) (fuseFile fuse.File, code fuse.Status) {
	writable := me.fileSystems[0]

	code = me.promoteDirsTo(name)
	if code != fuse.OK {
		return nil, code
	}
	fuseFile, code = writable.Create(name, flags, mode, context)
	if code.Ok() {
		fuseFile = me.newUnionFsFile(fuseFile, 0)
		me.removeDeletion(name)

		now := time.Nanoseconds()
		a := os.FileInfo{
			Mode:     fuse.S_IFREG | mode,
			Ctime_ns: now,
			Mtime_ns: now,
		}
		me.branchCache.Set(name, branchResult{&a, fuse.OK, 0})
	}
	return fuseFile, code
}

func (me *UnionFs) GetAttr(name string, context *fuse.Context) (a *os.FileInfo, s fuse.Status) {
	if name == _READONLY {
		return nil, fuse.ENOENT
	}
	if name == _DROP_CACHE {
		return &os.FileInfo{
			Mode: fuse.S_IFREG | 0777,
		}, fuse.OK
	}
	if name == me.options.DeletionDirName {
		return nil, fuse.ENOENT
	}
	isDel, s := me.isDeleted(name)
	if !s.Ok() {
		return nil, s
	}

	if isDel {
		return nil, fuse.ENOENT
	}
	r := me.getBranch(name)
	if r.branch < 0 {
		return nil, fuse.ENOENT
	}
	fi := *r.attr
	// Make everything appear writable.
	fi.Mode |= 0200
	return &fi, r.code
}

func (me *UnionFs) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	if name == _DROP_CACHE {
		return nil, fuse.ENODATA
	}

	r := me.getBranch(name)
	if r.branch >= 0 {
		return me.fileSystems[r.branch].GetXAttr(name, attr, context)
	}
	return nil, fuse.ENOENT
}

func (me *UnionFs) OpenDir(directory string, context *fuse.Context) (stream chan fuse.DirEntry, status fuse.Status) {
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
				ch, s := pfs.OpenDir(directory, context)
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
	if deletions == nil {
		_, code := me.fileSystems[0].GetAttr(me.options.DeletionDirName, context)
		if code == fuse.ENOENT {
			deletions = map[string]bool{}
		} else {
			return nil, syscall.EROFS
		}
	}

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

// recursivePromote promotes path, and if a directory, everything
// below that directory.  It returns a list of all promoted paths, in
// full, including the path itself.
func (me *UnionFs) recursivePromote(path string, pathResult branchResult, context *fuse.Context) (names []string, code fuse.Status) {
	names = []string{}
	if pathResult.branch > 0 {
		code = me.Promote(path, pathResult, context)
	}

	if code.Ok() {
		names = append(names, path)
	}

	if code.Ok() && pathResult.attr != nil && pathResult.attr.IsDirectory() {
		var stream chan fuse.DirEntry
		stream, code = me.OpenDir(path, context)
		for e := range stream {
			if !code.Ok() {
				break
			}
			subnames := []string{}
			p := filepath.Join(path, e.Name)
			r := me.getBranch(p)
			subnames, code = me.recursivePromote(p, r, context)
			names = append(names, subnames...)
		}
	}

	if !code.Ok() {
		names = nil
	}
	return names, code
}

func (me *UnionFs) renameDirectory(srcResult branchResult, srcDir string, dstDir string, context *fuse.Context) (code fuse.Status) {
	names := []string{}
	if code.Ok() {
		names, code = me.recursivePromote(srcDir, srcResult, context)
	}
	if code.Ok() {
		code = me.promoteDirsTo(dstDir)
	}

	if code.Ok() {
		writable := me.fileSystems[0]
		code = writable.Rename(srcDir, dstDir, context)
	}

	if code.Ok() {
		for _, srcName := range names {
			relative := strings.TrimLeft(srcName[len(srcDir):], string(filepath.Separator))
			dst := filepath.Join(dstDir, relative)
			me.removeDeletion(dst)

			srcResult := me.getBranch(srcName)
			srcResult.branch = 0
			me.branchCache.Set(dst, srcResult)

			srcResult = me.branchCache.GetFresh(srcName).(branchResult)
			if srcResult.branch > 0 {
				code = me.putDeletion(srcName)
			}
		}
	}
	return code
}

func (me *UnionFs) Rename(src string, dst string, context *fuse.Context) (code fuse.Status) {
	srcResult := me.getBranch(src)
	code = srcResult.code
	if code.Ok() {
		code = srcResult.code
	}

	if srcResult.attr.IsDirectory() {
		return me.renameDirectory(srcResult, src, dst, context)
	}

	if code.Ok() && srcResult.branch > 0 {
		code = me.Promote(src, srcResult, context)
	}
	if code.Ok() {
		code = me.promoteDirsTo(dst)
	}
	if code.Ok() {
		code = me.fileSystems[0].Rename(src, dst, context)
	}

	if code.Ok() {
		me.removeDeletion(dst)
		// Rename is racy; avoid racing with unionFsFile.Release().
		me.branchCache.DropEntry(dst)

		srcResult := me.branchCache.GetFresh(src)
		if srcResult.(branchResult).branch > 0 {
			code = me.putDeletion(src)
		}
	}
	return code
}

func (me *UnionFs) DropBranchCache(names []string) {
	me.branchCache.DropAll(names)
}

func (me *UnionFs) DropDeletionCache() {
	me.deletionCache.DropCache()
}

func (me *UnionFs) DropSubFsCaches() {
	for _, fs := range me.fileSystems {
		a, code := fs.GetAttr(_DROP_CACHE, nil)
		if code.Ok() && a.IsRegular() {
			f, _ := fs.Open(_DROP_CACHE, uint32(os.O_WRONLY), nil)
			if f != nil {
				f.Flush()
				f.Release()
			}
		}
	}
}

func (me *UnionFs) Open(name string, flags uint32, context *fuse.Context) (fuseFile fuse.File, status fuse.Status) {
	if name == _DROP_CACHE {
		if flags&fuse.O_ANYWRITE != 0 {
			log.Println("Forced cache drop on", me)
			me.DropBranchCache(nil)
			me.DropDeletionCache()
			me.DropSubFsCaches()
			me.nodeFs.ForgetClientInodes()
		}
		return fuse.NewDevNullFile(), fuse.OK
	}
	r := me.getBranch(name)
	if r.branch < 0 {
		// This should not happen, as a GetAttr() should have
		// already verified existence.
		log.Println("UnionFs: open of non-existent file:", name)
		return nil, fuse.ENOENT
	}
	if flags&fuse.O_ANYWRITE != 0 && r.branch > 0 {
		code := me.Promote(name, r, context)
		if code != fuse.OK {
			return nil, code
		}
		r.branch = 0
		r.attr.Mtime_ns = time.Nanoseconds()
		me.branchCache.Set(name, r)
	}
	fuseFile, status = me.fileSystems[r.branch].Open(name, uint32(flags), context)
	if fuseFile != nil {
		fuseFile = me.newUnionFsFile(fuseFile, r.branch)
	}
	return fuseFile, status
}

func (me *UnionFs) String() string {
	names := []string{}
	for _, fs := range me.fileSystems {
		names = append(names, fs.String())
	}
	return fmt.Sprintf("UnionFs(%v)", names)
}

func (me *UnionFs) StatFs(name string) *fuse.StatfsOut {
	return me.fileSystems[0].StatFs("")
}

type unionFsFile struct {
	fuse.File
	ufs   *UnionFs
	node  *fuse.Inode
	layer int
}

func (me *unionFsFile) String() string {
	return fmt.Sprintf("unionFsFile(%s)", me.File.String())
}

func (me *UnionFs) newUnionFsFile(f fuse.File, branch int) *unionFsFile {
	return &unionFsFile{
		File:  f,
		ufs:   me,
		layer: branch,
	}
}

func (me *unionFsFile) InnerFile() (file fuse.File) {
	return me.File
}

// We can't hook on Release. Release has no response, so it is not
// ordered wrt any following calls.
func (me *unionFsFile) Flush() (code fuse.Status) {
	code = me.File.Flush()
	path := me.ufs.nodeFs.Path(me.node)
	me.ufs.branchCache.GetFresh(path)
	return code
}

func (me *unionFsFile) SetInode(node *fuse.Inode) {
	me.node = node
}

func (me *unionFsFile) GetAttr() (*os.FileInfo, fuse.Status) {
	fi, code := me.File.GetAttr()
	if fi != nil {
		f := *fi
		fi = &f
		fi.Mode |= 0200
	}
	return fi, code
}
