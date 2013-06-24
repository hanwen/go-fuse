package nodefs

import (
	"github.com/hanwen/go-fuse/fuse"
)
	
func MountFileSystem(mountpoint string, nodeFs FileSystem, opts *Options) (*fuse.MountState, *FileSystemConnector, error) {
	conn := NewFileSystemConnector(nodeFs, opts)
	mountState := fuse.NewMountState(conn.RawFS())
	err := mountState.Mount(mountpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	return mountState, conn, nil
}
