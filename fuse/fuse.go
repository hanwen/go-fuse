package fuse

import "log"

var _ = log.Println

func MountNodeFileSystem(mountpoint string, nodeFs NodeFileSystem, opts *FileSystemOptions) (*MountState, *FileSystemConnector, error) {
	conn := NewFileSystemConnector(nodeFs, opts)
	mountState := NewMountState(conn)
	err := mountState.Mount(mountpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	return mountState, conn, nil
}

func MountPathFileSystem(mountpoint string, pathFs FileSystem, opts *FileSystemOptions) (*MountState, *FileSystemConnector, error) {
	nfs := NewPathNodeFs(pathFs, nil)
	return MountNodeFileSystem(mountpoint, nfs, opts)
}
