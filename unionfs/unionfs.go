package unionfs

import (
	"crypto/md5"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"io"
	"io/ioutil"
	"log"
	"os"
	"syscall"
	"path"
	"path/filepath"
	"sync"
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

 * It piggybacks on the existing LoopbackFileSystem in Go-fuse, so we
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

	branches []*fuse.LoopbackFileSystem

	// The same, but as interfaces.
	fileSystems []fuse.FileSystem

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

func NewUnionFs(roots []string, options UnionFsOptions) *UnionFs {
	g := new(UnionFs)

	g.options = &options
	for _, r := range roots {
		pt := fuse.NewLoopbackFileSystem(r)
		g.branches = append(g.branches, pt)

		// We could use some sort of caching file system here.
		g.fileSystems = append(g.fileSystems, fuse.FileSystem(pt))
	}

	deletionDir := g.deletionDir()
	err := os.MkdirAll(deletionDir, 0755)
	if err != nil {
		panic(fmt.Sprintf("could not create deletion path %v: %v",
			deletionDir, err))
	}

	g.deletionCache = NewDirCache(deletionDir, int64(options.DeletionCacheTTLSecs*1e9))
	g.branchCache = NewTimedCache(
		func(n string) interface{} { return g.getBranchAttrNoCache(n) },
		int64(options.BranchCacheTTLSecs*1e9))
	g.branchCache.RecurringPurge()
	return g
}

////////////////
// Deal with all the caches.

func (me *UnionFs) isDeleted(name string) bool {
	haveCache, found := me.deletionCache.HasEntry(filePathHash(name))
	if haveCache {
		return found
	}

	fileName := me.deletionPath(name)
	fi, _ := os.Lstat(fileName)
	return fi != nil
}

func (me *UnionFs) getBranch(name string) int {
	r := me.branchCache.Get(name)
	result := r.(getBranchResult)
	return result.branch
}

type getBranchResult struct {
	attr   *fuse.Attr
	code   fuse.Status
	branch int
}

func (me *UnionFs) getBranchAttrNoCache(name string) getBranchResult {
	parent, base := path.Split(name)
	parentBranch := 0
	if base != "" {
		parentBranch = me.getBranch(parent)
	}

	for i, fs := range me.fileSystems {
		if i < parentBranch {
			continue
		}

		a, s := fs.GetAttr(name)
		if s == fuse.OK {
			return getBranchResult{
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
	return getBranchResult{nil, fuse.ENOENT, -1}
}

////////////////
// Deletion.

func (me *UnionFs) deletionDir() string {
	dir := filepath.Join(me.branches[0].GetPath(""), me.options.DeletionDirName)
	return dir
}

func (me *UnionFs) deletionPath(name string) string {
	dir := me.deletionDir()

	return filepath.Join(dir, filePathHash(name))
}

func (me *UnionFs) removeDeletion(name string) {
	fileName := me.deletionPath(name)
	me.deletionCache.RemoveEntry(path.Base(fileName))

	// os.Remove tries to be smart and issues a Remove() and
	// Rmdir() sequentially.  We want to skip the 2nd system call,
	// so use syscall.Unlink() directly.
	errno := syscall.Unlink(name)
	if errno != 0 && errno != syscall.ENOENT {
		log.Printf("error unlinking %s: %v", fileName, errno)
	}
}

func (me *UnionFs) putDeletion(name string) fuse.Status {
	fileName := me.deletionPath(name)
	me.deletionCache.AddEntry(path.Base(fileName))

	// Is there a WriteStringToFileOrDie ?
	err := ioutil.WriteFile(fileName, []byte(name), 0644)
	if err != nil {
		log.Printf("could not create deletion file %v: %v",
			fileName, err)
		return fuse.EPERM
	}

	return fuse.OK
}

////////////////
// Promotion.

// From the golang blog.
func CopyFile(dstName, srcName string) (written int64, err os.Error) {
	src, err := os.Open(srcName)
	if err != nil {
		return
	}
	defer src.Close()

	dir, _ := filepath.Split(dstName)
	fi, err := os.Stat(dir)
	if fi != nil && !fi.IsDirectory() {
		return 0, os.NewError("Destination is not a directory.")
	}

	if err != nil {
		// TODO - umask support.
		err = os.MkdirAll(dir, 0755)
	}
	if err != nil {
		return 0, err
	}

	dst, err := os.Create(dstName)
	if err != nil {
		return
	}
	defer dst.Close()

	return io.Copy(dst, src)
}

func (me *UnionFs) Promote(name string, src *fuse.LoopbackFileSystem) fuse.Status {
	writable := me.branches[0]
	_, err := CopyFile(writable.GetPath(name), src.GetPath(name))
	me.branchCache.Set(name, getBranchResult{nil, fuse.OK, 0})
	if err != nil {
		log.Println("promote error: ", name, err.String())
		return fuse.EPERM
	}

	return fuse.OK
}

////////////////////////////////////////////////////////////////
// Below: implement interface for a FileSystem.

func (me *UnionFs) Rmdir(path string) (code fuse.Status) {
	r := me.branchCache.Get(path).(getBranchResult)
	if r.code != fuse.OK {
		return r.code
	}
	if r.attr.Mode & fuse.S_IFDIR == 0 {
		return syscall.ENOTDIR
	}
	if r.branch > 0 {
		stream, code := me.fileSystems[r.branch].OpenDir(path)
		if code == fuse.OK {
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

	r = me.branchCache.getDataNoCache(path).(getBranchResult)
	if r.branch > 0 {
		code = me.putDeletion(path)
	}
	return code
}
	
func (me *UnionFs) Mkdir(path string, mode uint32) (code fuse.Status) {
	r := me.branchCache.Get(path).(getBranchResult)
	if r.code != fuse.ENOENT {
		return syscall.EEXIST
	}
	code = me.fileSystems[0].Mkdir(path, mode)
	if code == fuse.OK {
		me.removeDeletion(path)
		attr := &fuse.Attr{
			Mode: fuse.S_IFDIR | mode,
		}
		me.branchCache.Set(path, getBranchResult{attr, fuse.OK, 0})
	}
	return code
}

func (me *UnionFs) Symlink(pointedTo string, linkName string) (code fuse.Status) {
	code = me.fileSystems[0].Symlink(pointedTo, linkName)
	if code == fuse.OK {
		me.removeDeletion(linkName)
		me.branchCache.Set(linkName, getBranchResult{nil, fuse.OK, 0})
	}
	return code
}

func (me *UnionFs) Chmod(name string, mode uint32) (code fuse.Status) {
	r := me.branchCache.Get(name).(getBranchResult)
	if r.attr == nil || r.code != fuse.OK {
		return r.code
	}

	if r.attr.Mode & fuse.S_IFREG == 0 {
		return fuse.EPERM
	}

	permMask := uint32(07777)
	oldMode := r.attr.Mode & permMask

	if oldMode != mode {
		if r.branch > 0 {
			code := me.Promote(name, me.branches[r.branch])
			if code != fuse.OK {
				return code
			}
		}
		me.fileSystems[0].Chmod(name, mode)
	}
	return fuse.OK
}

func (me *UnionFs) Access(name string, mode uint32) (code fuse.Status) {
	i := me.getBranch(name)
	if i >= 0 {
		return me.fileSystems[i].Access(name, mode)
	}

	return fuse.ENOENT
}

func (me *UnionFs) Unlink(name string) (code fuse.Status) {
	branch := me.getBranch(name)
	if branch == 0 {
		code = me.fileSystems[0].Unlink(name)
		if code != fuse.OK {
			return code
		}
		r := me.branchCache.getDataNoCache(name)
		branch = r.(getBranchResult).branch
	}

	if branch > 0 {
		// It would be nice to do the putDeletion async.
		code = me.putDeletion(name)
	}
	return code
}

func (me *UnionFs) Readlink(name string) (out string, code fuse.Status) {
	i := me.getBranch(name)
	if i >= 0 {
		return me.fileSystems[i].Readlink(name)
	}
	return "", fuse.ENOENT
}

func (me *UnionFs) Create(name string, flags uint32, mode uint32) (fuseFile fuse.File, code fuse.Status) {
	// TODO(hanwen) - we should check that the name is not a
	// directory in another branch.
	writable := me.fileSystems[0]
	fuseFile, code = writable.Create(name, flags, mode)
	if code == fuse.OK {
		me.removeDeletion(name)
		me.branchCache.Set(name, getBranchResult{nil, fuse.OK, 0})
	}
	return fuseFile, code
}

func (me *UnionFs) GetAttr(name string) (a *fuse.Attr, s fuse.Status) {
	if name == _READONLY {
		return nil, fuse.ENOENT
	}
	if name == _DROP_CACHE {
		log.Println("Forced cache drop")
		me.branchCache.Purge()
		me.deletionCache.DropCache()
		return nil, fuse.ENOENT
	}
	if name == me.options.DeletionDirName {
		return nil, fuse.ENOENT
	}
	if me.isDeleted(name) {
		return nil, fuse.ENOENT
	}
	r := me.branchCache.Get(name).(getBranchResult)
	if r.branch < 0 {
		return nil, fuse.ENOENT
	}
	if r.attr == nil {
		return me.fileSystems[r.branch].GetAttr(name)
	}
	return r.attr, r.code
}

func (me *UnionFs) GetXAttr(name string, attr string) ([]byte, fuse.Status) {
	branch := me.getBranch(name)
	if branch >= 0 {
		return me.fileSystems[branch].GetXAttr(name, attr)
	}
	return nil, fuse.ENOENT
}

func (me *UnionFs) OpenDir(directory string) (stream chan fuse.DirEntry, status fuse.Status) {
	dirBranch := me.getBranch(directory)
	if dirBranch < 0 {
		return nil, fuse.ENOENT
	}

	// We could try to use the cache, but we have a delay, so
	// might as well get the fresh results async.
	var deletions map[string]bool
	deletionsDone := make(chan bool, 1)
	go func() {
		deletions = newDirnameMap(me.deletionDir())
		deletionsDone <- true
	}()

	entries := make([]map[string]uint32, len(me.branches))
	for i, _ := range me.branches {
		entries[i] = make(map[string]uint32)
	}

	statuses := make([]fuse.Status, len(me.branches))
	var wg sync.WaitGroup
	for i, l := range me.fileSystems {
		if i >= dirBranch {
			wg.Add(1)
			go func(j int, pfs fuse.FileSystem) {
				ch, s := pfs.OpenDir(directory)
				statuses[j] = s
				for s == fuse.OK {
					v := <-ch
					if v.Name == "" {
						break
					}
					entries[j][v.Name] = v.Mode
				}
				wg.Done()
			}(i, l)
		}
	}

	wg.Wait()
	_ = <-deletionsDone

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
		results["READONLY"] = 0, false
	}

	stream = make(chan fuse.DirEntry)
	go func() {
		for k, v := range results {
			stream <- fuse.DirEntry{
				Name: k,
				Mode: v,
			}
		}
		close(stream)
	}()
	return stream, fuse.OK
}

func (me *UnionFs) Rename(src string, dst string) (status fuse.Status) {
	srcResult := me.branchCache.Get(src).(getBranchResult)
	if srcResult.code != fuse.OK {
		return srcResult.code
	}

	if srcResult.branch > 0 {
		code := me.Promote(src, me.branches[srcResult.branch])
		if code != fuse.OK {
			return code
		}
	}
	code := me.fileSystems[0].Rename(src, dst)
	if code != fuse.OK {
		return code
	}

	me.removeDeletion(dst)
	srcResult.branch = 0
	me.branchCache.Set(dst, srcResult)
	
	if srcResult.branch == 0 {
		srcResult := me.branchCache.getDataNoCache(src)
		if srcResult.(getBranchResult).branch > 0 {
			code = me.putDeletion(src)
		}
	} else {
		code = me.putDeletion(src)
	}

	return code
}

func (me *UnionFs) Open(name string, flags uint32) (fuseFile fuse.File, status fuse.Status) {
	branch := me.getBranch(name)
	if flags&fuse.O_ANYWRITE != 0 && branch > 0 {
		code := me.Promote(name, me.branches[branch])
		if code != fuse.OK {
			return nil, code
		}
		branch = 0
	}

	return me.fileSystems[branch].Open(name, uint32(flags))
}

func (me *UnionFs) Roots() (result []string) {
	for _, loopback := range me.branches {
		result = append(result, loopback.GetPath(""))
	}
	return result
}
