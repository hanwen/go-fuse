package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/splice"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type knownFs struct {
	*UnionFs
	*fuse.PathNodeFs
}

// Creates unions for all files under a given directory,
// walking the tree and looking for directories D which have a
// D/READONLY symlink.
//
// A union for A/B/C will placed under directory A-B-C.
type AutoUnionFs struct {
	fuse.DefaultFileSystem

	lock             sync.RWMutex
	knownFileSystems map[string]knownFs
	nameRootMap      map[string]string
	root             string

	nodeFs  *fuse.PathNodeFs
	options *AutoUnionFsOptions

	mountState *fuse.MountState
	connector  *fuse.FileSystemConnector
}

type AutoUnionFsOptions struct {
	UnionFsOptions
	fuse.FileSystemOptions
	fuse.PathNodeFsOptions

	// If set, run updateKnownFses() after mounting.
	UpdateOnMount bool

	// If set hides the _READONLY file.
	HideReadonly bool
}

const (
	_READONLY    = "READONLY"
	_STATUS      = "status"
	_CONFIG      = "config"
	_DEBUG       = "debug"
	_DEBUG_SETTING = "debug_setting"
	_ROOT        = "root"
	_VERSION     = "gounionfs_version"
	_SCAN_CONFIG = ".scan_config"
)

func NewAutoUnionFs(directory string, options AutoUnionFsOptions) *AutoUnionFs {
	if options.HideReadonly {
		options.HiddenFiles = append(options.HiddenFiles, _READONLY)
	}
	a := new(AutoUnionFs)
	a.knownFileSystems = make(map[string]knownFs)
	a.nameRootMap = make(map[string]string)
	a.options = &options
	directory, err := filepath.Abs(directory)
	if err != nil {
		panic("filepath.Abs returned err")
	}
	a.root = directory
	return a
}

func (fs *AutoUnionFs) String() string {
	return fmt.Sprintf("AutoUnionFs(%s)", fs.root)
}

func (fs *AutoUnionFs) OnMount(nodeFs *fuse.PathNodeFs) {
	fs.nodeFs = nodeFs
	if fs.options.UpdateOnMount {
		time.AfterFunc(100*time.Millisecond, func() { fs.updateKnownFses() })
	}
}

func (fs *AutoUnionFs) addAutomaticFs(roots []string) {
	relative := strings.TrimLeft(strings.Replace(roots[0], fs.root, "", -1), "/")
	name := strings.Replace(relative, "/", "-", -1)

	if fs.getUnionFs(name) == nil {
		fs.addFs(name, roots)
	}
}

func (fs *AutoUnionFs) createFs(name string, roots []string) fuse.Status {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	for workspace, root := range fs.nameRootMap {
		if root == roots[0] && workspace != name {
			log.Printf("Already have a union FS for directory %s in workspace %s",
				roots[0], workspace)
			return fuse.EBUSY
		}
	}

	known := fs.knownFileSystems[name]
	if known.UnionFs != nil {
		log.Println("Already have a workspace:", name)
		return fuse.EBUSY
	}

	ufs, err := NewUnionFsFromRoots(roots, &fs.options.UnionFsOptions, true)
	if err != nil {
		log.Println("Could not create UnionFs:", err)
		return fuse.EPERM
	}

	log.Printf("Adding workspace %v for roots %v", name, ufs.String())
	nfs := fuse.NewPathNodeFs(ufs, &fs.options.PathNodeFsOptions)
	code := fs.nodeFs.Mount(name, nfs, &fs.options.FileSystemOptions)
	if code.Ok() {
		fs.knownFileSystems[name] = knownFs{
			ufs,
			nfs,
		}
		fs.nameRootMap[name] = roots[0]
	}
	return code
}

func (fs *AutoUnionFs) rmFs(name string) (code fuse.Status) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	known := fs.knownFileSystems[name]
	if known.UnionFs == nil {
		return fuse.ENOENT
	}

	code = fs.nodeFs.Unmount(name)
	if code.Ok() {
		delete(fs.knownFileSystems, name)
		delete(fs.nameRootMap, name)
	} else {
		log.Printf("Unmount failed for %s.  Code %v", name, code)
	}

	return code
}

func (fs *AutoUnionFs) addFs(name string, roots []string) (code fuse.Status) {
	if name == _CONFIG || name == _STATUS || name == _SCAN_CONFIG {
		log.Printf("Illegal name %q for overlay: %v", name, roots)
		return fuse.EINVAL
	}
	return fs.createFs(name, roots)
}

func (fs *AutoUnionFs) getRoots(path string) []string {
	ro := filepath.Join(path, _READONLY)
	fi, err := os.Lstat(ro)
	fiDir, errDir := os.Stat(ro)
	if err != nil || errDir != nil {
		return nil
	}

	if fi.Mode()&os.ModeSymlink != 0 && fiDir.IsDir() {
		// TODO - should recurse and chain all READONLYs
		// together.
		return []string{path, ro}
	}
	return nil
}

func (fs *AutoUnionFs) visit(path string, fi os.FileInfo, err error) error {
	if fi != nil && fi.IsDir() {
		roots := fs.getRoots(path)
		if roots != nil {
			fs.addAutomaticFs(roots)
		}
	}
	return nil
}

func (fs *AutoUnionFs) updateKnownFses() {
	log.Println("Looking for new filesystems")
	// We unroll the first level of entries in the root manually in order
	// to allow symbolic links on that level.
	directoryEntries, err := ioutil.ReadDir(fs.root)
	if err == nil {
		for _, dir := range directoryEntries {
			if dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
				path := filepath.Join(fs.root, dir.Name())
				dir, _ = os.Stat(path)
				fs.visit(path, dir, nil)
				filepath.Walk(path,
					func(path string, fi os.FileInfo, err error) error {
						return fs.visit(path, fi, err)
					})
			}
		}
	}
	log.Println("Done looking")
}

func (fs *AutoUnionFs) Readlink(path string, context *fuse.Context) (out string, code fuse.Status) {
	comps := strings.Split(path, string(filepath.Separator))
	if comps[0] == _STATUS && comps[1] == _ROOT {
		return fs.root, fuse.OK
	}

	if comps[0] == _STATUS && comps[1] == _DEBUG_SETTING && fs.hasDebug() {
		return "1", fuse.OK
	}

	if comps[0] != _CONFIG {
		return "", fuse.ENOENT
	}

	name := comps[1]

	fs.lock.RLock()
	defer fs.lock.RUnlock()

	root, ok := fs.nameRootMap[name]
	if ok {
		return root, fuse.OK
	}
	return "", fuse.ENOENT
}

func (fs *AutoUnionFs) getUnionFs(name string) *UnionFs {
	fs.lock.RLock()
	defer fs.lock.RUnlock()
	return fs.knownFileSystems[name].UnionFs
}

func (fs *AutoUnionFs) Symlink(pointedTo string, linkName string, context *fuse.Context) (code fuse.Status) {
	comps := strings.Split(linkName, "/")
	if len(comps) != 2 {
		return fuse.EPERM
	}

	if comps[0] == _STATUS && comps[1] == _DEBUG_SETTING {
		fs.SetDebug(true)
		return fuse.OK
	}

	if comps[0] == _CONFIG {
		roots := fs.getRoots(pointedTo)
		if roots == nil {
			return fuse.Status(syscall.ENOTDIR)
		}

		name := comps[1]
		return fs.addFs(name, roots)
	}
	return fuse.EPERM
}

func (fs *AutoUnionFs) SetDebug(b bool) {
	// Officially, this should use locking, but we don't care
	// about race conditions here.
	fs.nodeFs.Debug = b
	fs.connector.Debug = b
	fs.mountState.Debug = b
}

func (fs *AutoUnionFs) hasDebug() bool {
	return fs.nodeFs.Debug
}

func (fs *AutoUnionFs) Unlink(path string, context *fuse.Context) (code fuse.Status) {
	comps := strings.Split(path, "/")
	if len(comps) != 2 {
		return fuse.EPERM
	}

	if comps[0] == _STATUS && comps[1] == _DEBUG_SETTING {
		fs.SetDebug(false)
		return fuse.OK
	}

	if comps[0] == _CONFIG && comps[1] != _SCAN_CONFIG {
		code = fs.rmFs(comps[1])
	} else {
		code = fuse.ENOENT
	}
	return code
}

// Must define this, because ENOSYS will suspend all GetXAttr calls.
func (fs *AutoUnionFs) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	return nil, fuse.ENODATA
}

func (fs *AutoUnionFs) GetAttr(path string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	if path == "" || path == _CONFIG || path == _STATUS {
		a := &fuse.Attr{
			Mode: fuse.S_IFDIR | 0755,
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_STATUS, _DEBUG_SETTING) && fs.hasDebug() {
		return &fuse.Attr{
			Mode: fuse.S_IFLNK | 0644,
		}, fuse.OK
	}

	if path == filepath.Join(_STATUS, _VERSION) {
		a := &fuse.Attr{
			Mode: fuse.S_IFREG | 0644,
			Size: uint64(len(fuse.Version())),
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_STATUS, _DEBUG) {
		a := &fuse.Attr{
			Mode: fuse.S_IFREG | 0644,
			Size: uint64(len(fs.DebugData())),
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_STATUS, _ROOT) {
		a := &fuse.Attr{
			Mode: syscall.S_IFLNK | 0644,
		}
		return a, fuse.OK
	}

	if path == filepath.Join(_CONFIG, _SCAN_CONFIG) {
		a := &fuse.Attr{
			Mode: fuse.S_IFREG | 0644,
		}
		return a, fuse.OK
	}
	comps := strings.Split(path, string(filepath.Separator))

	if len(comps) > 1 && comps[0] == _CONFIG {
		fs := fs.getUnionFs(comps[1])

		if fs == nil {
			return nil, fuse.ENOENT
		}

		a := &fuse.Attr{
			Mode: syscall.S_IFLNK | 0644,
		}
		return a, fuse.OK
	}

	return nil, fuse.ENOENT
}

func (fs *AutoUnionFs) StatusDir() (stream []fuse.DirEntry, status fuse.Status) {
	stream = make([]fuse.DirEntry, 0, 10)
	stream = []fuse.DirEntry{
		{Name: _VERSION, Mode: fuse.S_IFREG | 0644},
		{Name: _DEBUG, Mode: fuse.S_IFREG | 0644},
		{Name: _ROOT, Mode: syscall.S_IFLNK | 0644},
	}
	if fs.hasDebug() {
		stream = append(stream, fuse.DirEntry{Name: _DEBUG_SETTING, Mode: fuse.S_IFLNK | 0644})
	}
	return stream, fuse.OK
}

// SetMountState stores the MountState, which is necessary for
// retrieving debug data.
func (fs *AutoUnionFs) SetMountState(state *fuse.MountState) {
	fs.mountState = state
}

func (fs *AutoUnionFs) SetFileSystemConnector(conn *fuse.FileSystemConnector) {
	fs.connector = conn
}

func (fs *AutoUnionFs) DebugData() string {
	if fs.mountState == nil {
		return "AutoUnionFs.mountState not set"
	}
	setting := fs.mountState.KernelSettings()
	msg := fmt.Sprintf(
		"Version: %v\n"+
			"Bufferpool: %v\n"+
			"Kernel: %v\n",
		fuse.Version(),
		fs.mountState.BufferPoolStats(),
		&setting)

	if fs.connector != nil {
		msg += fmt.Sprintf("Live inodes: %d\n", fs.connector.InodeHandleCount())
	}
	pairs := splice.Total()
	if pairs > 0 {
		msg += fmt.Sprintf("Pipes: %d\n", pairs)
	}

	return msg
}

func (fs *AutoUnionFs) Open(path string, flags uint32, context *fuse.Context) (fuse.File, fuse.Status) {
	if path == filepath.Join(_STATUS, _DEBUG) {
		if flags&fuse.O_ANYWRITE != 0 {
			return nil, fuse.EPERM
		}

		return fuse.NewDataFile([]byte(fs.DebugData())), fuse.OK
	}
	if path == filepath.Join(_STATUS, _VERSION) {
		if flags&fuse.O_ANYWRITE != 0 {
			return nil, fuse.EPERM
		}
		return fuse.NewDataFile([]byte(fuse.Version())), fuse.OK
	}
	if path == filepath.Join(_CONFIG, _SCAN_CONFIG) {
		if flags&fuse.O_ANYWRITE != 0 {
			fs.updateKnownFses()
		}
		return fuse.NewDevNullFile(), fuse.OK
	}
	return nil, fuse.ENOENT
}

func (fs *AutoUnionFs) Truncate(name string, offset uint64, context *fuse.Context) (code fuse.Status) {
	if name != filepath.Join(_CONFIG, _SCAN_CONFIG) {
		log.Println("Huh? Truncating unsupported write file", name)
		return fuse.EPERM
	}
	return fuse.OK
}

func (fs *AutoUnionFs) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, status fuse.Status) {
	switch name {
	case _STATUS:
		return fs.StatusDir()
	case _CONFIG:
	case "/":
		name = ""
	case "":
	default:
		log.Printf("Argh! Don't know how to list dir %v", name)
		return nil, fuse.ENOSYS
	}

	fs.lock.RLock()
	defer fs.lock.RUnlock()

	stream = make([]fuse.DirEntry, 0, len(fs.knownFileSystems)+5)
	if name == _CONFIG {
		for k := range fs.knownFileSystems {
			stream = append(stream, fuse.DirEntry{
				Name: k,
				Mode: syscall.S_IFLNK | 0644,
			})
		}
	}

	if name == "" {
		stream = append(stream, fuse.DirEntry{
			Name: _CONFIG,
			Mode: uint32(fuse.S_IFDIR | 0755),
		},
			fuse.DirEntry{
				Name: _STATUS,
				Mode: uint32(fuse.S_IFDIR | 0755),
			})
	}
	return stream, status
}

func (fs *AutoUnionFs) StatFs(name string) *fuse.StatfsOut {
	return &fuse.StatfsOut{}
}
