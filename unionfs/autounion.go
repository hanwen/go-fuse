package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"sync"
	"time"
	"io/ioutil"
)

// Creates unions for all files under a given directory,
// walking the tree and looking for directories D which have a
// D/READONLY symlink.
//
// A union for A/B/C will placed under directory A-B-C.
type AutoUnionFs struct {
	fuse.DefaultFileSystem

	lock             sync.RWMutex
	knownFileSystems map[string]*UnionFs
	nameRootMap      map[string]string
	root             string

	connector *fuse.FileSystemConnector

	options *AutoUnionFsOptions
}

type AutoUnionFsOptions struct {
	UnionFsOptions
	fuse.FileSystemOptions

	// If set, run updateKnownFses() after mounting.
	UpdateOnMount bool
}

const (
	_READONLY    = "READONLY"
	_STATUS      = "status"
	_CONFIG      = "config"
	_ROOT        = "root"
	_VERSION     = "gounionfs_version"
	_SCAN_CONFIG = ".scan_config"
)

func NewAutoUnionFs(directory string, options AutoUnionFsOptions) *AutoUnionFs {
	a := new(AutoUnionFs)
	a.knownFileSystems = make(map[string]*UnionFs)
	a.nameRootMap = make(map[string]string)
	a.options = &options
	directory, err := filepath.Abs(directory)
	if err != nil {
		panic("filepath.Abs returned err")
	}
	a.root = directory
	return a
}

func (me *AutoUnionFs) Name() string {
	return fmt.Sprintf("AutoUnionFs(%s)", me.root)
}

func (me *AutoUnionFs) Mount(connector *fuse.FileSystemConnector) {
	me.connector = connector
	if me.options.UpdateOnMount {
		time.AfterFunc(0.1e9, func() { me.updateKnownFses() })
	}
}

func (me *AutoUnionFs) addAutomaticFs(roots []string) {
	relative := strings.TrimLeft(strings.Replace(roots[0], me.root, "", -1), "/")
	name := strings.Replace(relative, "/", "-", -1)

	if me.getUnionFs(name) == nil {
		me.addFs(name, roots)
	}
}

func (me *AutoUnionFs) createFs(name string, roots []string) fuse.Status {
	me.lock.Lock()
	defer me.lock.Unlock()

	for workspace, root := range me.nameRootMap {
		if root == roots[0] && workspace != name {
			log.Printf("Already have a union FS for directory %s in workspace %s",
				roots[0], workspace)
			return fuse.EBUSY
		}
	}

	ufs := me.knownFileSystems[name]
	if ufs != nil {
		log.Println("Already have a workspace:", name)
		return fuse.EBUSY
	}

	ufs, err := NewUnionFsFromRoots(roots, &me.options.UnionFsOptions, true)
	if err != nil {
		log.Println("Could not create UnionFs:", err)
		return fuse.EPERM
	}

	log.Printf("Adding workspace %v for roots %v", name, ufs.Name())
	code := me.connector.Mount("/"+name, ufs, &me.options.FileSystemOptions)
	if code.Ok() {
		me.knownFileSystems[name] = ufs
		me.nameRootMap[name] = roots[0]
	}
	return code
}

func (me *AutoUnionFs) rmFs(name string) (code fuse.Status) {
	me.lock.Lock()
	defer me.lock.Unlock()

	fs := me.knownFileSystems[name]
	if fs == nil {
		return fuse.ENOENT
	}

	code = me.connector.Unmount(name)
	if code.Ok() {
		me.knownFileSystems[name] = nil, false
		me.nameRootMap[name] = "", false
	} else {
		log.Printf("Unmount failed for %s.  Code %v", name, code)
	}

	return code
}

func (me *AutoUnionFs) addFs(name string, roots []string) (code fuse.Status) {
	if name == _CONFIG || name == _STATUS || name == _SCAN_CONFIG {
		log.Println("Illegal name for overlay", roots)
		return fuse.EINVAL
	}
	return me.createFs(name, roots)
}

// TODO - should hide these methods.
func (me *AutoUnionFs) VisitDir(path string, f *os.FileInfo) bool {
	roots := me.getRoots(path)
	if roots != nil {
		me.addAutomaticFs(roots)
	}
	return true
}

func (me *AutoUnionFs) getRoots(path string) []string {
	ro := filepath.Join(path, _READONLY)
	fi, err := os.Lstat(ro)
	fiDir, errDir := os.Stat(ro)
	if err == nil && errDir == nil && fi.IsSymlink() && fiDir.IsDirectory() {
		// TODO - should recurse and chain all READONLYs
		// together.
		return []string{path, ro}
	}
	return nil
}

func (me *AutoUnionFs) VisitFile(path string, f *os.FileInfo) {

}

func (me *AutoUnionFs) updateKnownFses() {
	log.Println("Looking for new filesystems")
	// We unroll the first level of entries in the root manually in order
	// to allow symbolic links on that level.
	directoryEntries, err := ioutil.ReadDir(me.root)
	if err == nil {
		for _, dir := range directoryEntries {
			if dir.IsDirectory() || dir.IsSymlink() {
				path := filepath.Join(me.root, dir.Name)
				me.VisitDir(path, nil)
				filepath.Walk(path, me, nil)
			}
		}
	}
	log.Println("Done looking")
}

func (me *AutoUnionFs) Readlink(path string, context *fuse.Context) (out string, code fuse.Status) {
	comps := strings.Split(path, fuse.SeparatorString)
	if comps[0] == _STATUS && comps[1] == _ROOT {
		return me.root, fuse.OK
	}

	if comps[0] != _CONFIG {
		return "", fuse.ENOENT
	}
	name := comps[1]
	me.lock.RLock()
	defer me.lock.RUnlock()

	root, ok := me.nameRootMap[name]
	if ok {
		return root, fuse.OK
	}
	return "", fuse.ENOENT
}

func (me *AutoUnionFs) getUnionFs(name string) *UnionFs {
	me.lock.RLock()
	defer me.lock.RUnlock()
	return me.knownFileSystems[name]
}

func (me *AutoUnionFs) Symlink(pointedTo string, linkName string, context *fuse.Context) (code fuse.Status) {
	comps := strings.Split(linkName, "/")
	if len(comps) != 2 {
		return fuse.EPERM
	}

	if comps[0] == _CONFIG {
		roots := me.getRoots(pointedTo)
		if roots == nil {
			return syscall.ENOTDIR
		}

		name := comps[1]
		return me.addFs(name, roots)
	}
	return fuse.EPERM
}

func (me *AutoUnionFs) Unlink(path string, context *fuse.Context) (code fuse.Status) {
	comps := strings.Split(path, "/")
	if len(comps) != 2 {
		return fuse.EPERM
	}

	if comps[0] == _CONFIG && comps[1] != _SCAN_CONFIG {
		code = me.rmFs(comps[1])
	} else {
		code = fuse.ENOENT
	}
	return code
}

// Must define this, because ENOSYS will suspend all GetXAttr calls.
func (me *AutoUnionFs) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	return nil, fuse.ENODATA
}

func (me *AutoUnionFs) GetAttr(path string, context *fuse.Context) (*os.FileInfo, fuse.Status) {
	if path == "" || path == _CONFIG || path == _STATUS {
		a := &os.FileInfo{
			Mode: fuse.S_IFDIR | 0755,
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_STATUS, _VERSION) {
		a := &os.FileInfo{
			Mode: fuse.S_IFREG | 0644,
			Size: int64(len(fuse.Version())),
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_STATUS, _ROOT) {
		a := &os.FileInfo{
			Mode: syscall.S_IFLNK | 0644,
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_CONFIG, _SCAN_CONFIG) {
		a := &os.FileInfo{
			Mode: fuse.S_IFREG | 0644,
		}
		return a, fuse.OK
	}
	comps := strings.Split(path, fuse.SeparatorString)

	if len(comps) > 1 && comps[0] == _CONFIG {
		fs := me.getUnionFs(comps[1])

		if fs == nil {
			return nil, fuse.ENOENT
		}

		a := &os.FileInfo{
			Mode: syscall.S_IFLNK | 0644,
		}
		return a, fuse.OK
	}

	return nil, fuse.ENOENT
}

func (me *AutoUnionFs) StatusDir() (stream chan fuse.DirEntry, status fuse.Status) {
	stream = make(chan fuse.DirEntry, 10)
	stream <- fuse.DirEntry{
		Name: _VERSION,
		Mode: fuse.S_IFREG | 0644,
	}
	stream <- fuse.DirEntry{
		Name: _ROOT,
		Mode: syscall.S_IFLNK | 0644,
	}

	close(stream)
	return stream, fuse.OK
}

func (me *AutoUnionFs) Open(path string, flags uint32, context *fuse.Context) (fuse.File, fuse.Status) {
	if path == filepath.Join(_STATUS, _VERSION) {
		if flags&fuse.O_ANYWRITE != 0 {
			return nil, fuse.EPERM
		}
		return fuse.NewReadOnlyFile([]byte(fuse.Version())), fuse.OK
	}
	if path == filepath.Join(_CONFIG, _SCAN_CONFIG) {
		if flags&fuse.O_ANYWRITE != 0 {
			me.updateKnownFses()
		}
		return fuse.NewDevNullFile(), fuse.OK
	}
	return nil, fuse.ENOENT
}

func (me *AutoUnionFs) Truncate(name string, offset uint64, context *fuse.Context) (code fuse.Status) {
	if name != filepath.Join(_CONFIG, _SCAN_CONFIG) {
		log.Println("Huh? Truncating unsupported write file", name)
		return fuse.EPERM
	}
	return fuse.OK
}

func (me *AutoUnionFs) OpenDir(name string, context *fuse.Context) (stream chan fuse.DirEntry, status fuse.Status) {
	switch name {
	case _STATUS:
		return me.StatusDir()
	case _CONFIG:
	case "/":
		name = ""
	case "":
	default:
		log.Printf("Argh! Don't know how to list dir %v", name)
		return nil, fuse.ENOSYS
	}

	me.lock.RLock()
	defer me.lock.RUnlock()

	stream = make(chan fuse.DirEntry, len(me.knownFileSystems)+5)
	if name == _CONFIG {
		for k, _ := range me.knownFileSystems {
			stream <- fuse.DirEntry{
				Name: k,
				Mode: syscall.S_IFLNK | 0644,
			}
		}
	}

	if name == "" {
		stream <- fuse.DirEntry{
			Name: _CONFIG,
			Mode: uint32(fuse.S_IFDIR | 0755),
		}
		stream <- fuse.DirEntry{
			Name: _STATUS,
			Mode: uint32(fuse.S_IFDIR | 0755),
		}
	}
	close(stream)
	return stream, status
}

func (me *AutoUnionFs) StatFs() *fuse.StatfsOut {
	return &fuse.StatfsOut{}
}
