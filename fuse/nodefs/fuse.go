package nodefs

import (
	"github.com/hanwen/go-fuse/fuse"
)

// Mounts a filesystem with the given root node on the given directory
func MountRoot(mountpoint string, root Node, opts *Options) (*fuse.Server, *FileSystemConnector, error) {
	conn := NewFileSystemConnector(root, opts)
	s, err := fuse.NewServer(conn.RawFS(), mountpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	return s, conn, nil
}

// Mounts a filesystem on the given directory
func MountFileSystem(mountpoint string, fs FileSystem, opts *Options) (*fuse.Server, *FileSystemConnector, error) {
	return MountRoot(mountpoint, fs.Root(), opts)
}
