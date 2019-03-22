// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// Mount mounts the given NodeFS on the directory, and starts serving
// requests. This is a convenience wrapper around NewNodeFS and
// fuse.NewServer.  If nil is given as options, default settings are
// applied, which are 1 second entry and attribute timeout.
func Mount(dir string, root DirOperations, options *Options) (*fuse.Server, error) {
	if options == nil {
		oneSec := time.Second
		options = &Options{
			EntryTimeout: &oneSec,
			AttrTimeout:  &oneSec,
		}
	}

	rawFS := NewNodeFS(root, options)
	server, err := fuse.NewServer(rawFS, dir, &options.MountOptions)
	if err != nil {
		return nil, err
	}

	go server.Serve()
	if err := server.WaitMount(); err != nil {
		// XXX should shutdown the serve loop?
		return nil, err
	}

	return server, nil
}
