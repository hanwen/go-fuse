package unionfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"os"
)

func NewUnionFsFromRoots(roots []string, opts *UnionFsOptions, roCaching bool) (*UnionFs, os.Error) {
	fses := make([]fuse.FileSystem, 0)
	for i, r := range roots {
		var fs fuse.FileSystem
		fi, err := os.Stat(r)
		if err != nil {
			return nil, err
		}
		if fi.IsDirectory() {
			fs = fuse.NewLoopbackFileSystem(r)
		}
		if fs == nil {
			return nil, err

		}
		if i > 0 && roCaching {
			fs = NewCachingFileSystem(fs, 0)
		}

		fses = append(fses, fs)
	}

	return NewUnionFs(fses, *opts), nil
}
