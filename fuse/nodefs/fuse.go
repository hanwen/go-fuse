package nodefs

import (
	"github.com/hanwen/go-fuse/fuse"
)

func MountFileSystem(mountpoint string, nodeFs FileSystem, opts *Options) (*fuse.Server, *FileSystemConnector, error) {
	conn := NewFileSystemConnector(nodeFs, opts)
	s, err := fuse.NewServer(conn.RawFS(), mountpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	return s, conn, nil
}
