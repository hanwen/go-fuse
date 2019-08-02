// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Mount mounts a filesystem with the given root node on the given directory.
// Convenience wrapper around fuse.NewServer
func Mount(mountpoint string, root Node, mountOptions *fuse.MountOptions, nodefsOptions *Options) (*fuse.Server, *FileSystemConnector, error) {
	conn := NewFileSystemConnector(root, nodefsOptions)
	s, err := fuse.NewServer(conn.RawFS(), mountpoint, mountOptions)
	if err != nil {
		return nil, nil, err
	}
	return s, conn, nil
}

// MountRoot is like Mount but uses default fuse mount options.
func MountRoot(mountpoint string, root Node, opts *Options) (*fuse.Server, *FileSystemConnector, error) {
	mountOpts := &fuse.MountOptions{}
	if opts != nil && opts.Debug {
		mountOpts.Debug = opts.Debug
	}
	return Mount(mountpoint, root, mountOpts, opts)
}
