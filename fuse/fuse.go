package fuse
import (
	"os"
	"fmt"
)

func MountFileSystem(mountpoint string, fs FileSystem, opts *FileSystemOptions) (*MountState, *FileSystemConnector, os.Error) {
	conn := NewFileSystemConnector(fs, opts)
	mountState := NewMountState(conn)
	fmt.Printf("Go-FUSE Version %v.\nMounting...\n", Version())
	err := mountState.Mount(mountpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("Mounted!")
	return mountState, conn, nil
}
