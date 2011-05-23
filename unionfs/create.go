package unionfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/zipfs"
	"os"
)

func NewUnionFsFromRoots(roots []string, opts *UnionFsOptions) (*UnionFs, os.Error) {
	fses := make([]fuse.FileSystem, 0)
	for _, r := range roots {
		var fs fuse.FileSystem
		fi, err := os.Stat(r)
		if err != nil {
			return nil, err
		}
		if fi.IsDirectory() {
			fs = fuse.NewLoopbackFileSystem(r)
		}
		if fs == nil {
			fs, err = zipfs.NewArchiveFileSystem(r)
		}
		if fs == nil {
			return nil, err
		}

		fses = append(fses, fs)
	}

	identifier := fmt.Sprintf("%v", roots)
	return NewUnionFs(identifier, fses, *opts), nil
}
