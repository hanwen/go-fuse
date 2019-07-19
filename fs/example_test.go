// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/hanwen/go-fuse/fs"
	"github.com/hanwen/go-fuse/fuse"
)

// An example of creating a loopback file system, and mounting it onto
// a directory
func Example_mountLoopback() {
	mntDir, _ := ioutil.TempDir("", "")

	// Make $HOME available on a mount dir under /tmp/ . Caution:
	// write operations are also mirrored.
	root, err := fs.NewLoopbackRoot(os.Getenv("HOME"))
	if err != nil {
		log.Fatal(err)
	}

	// Mount the file system
	server, err := fs.Mount(mntPoint, root, &fs.Options{
		MountOptions: fuse.MountOptions{Debug: true},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Mounted %s as loopback on %s\n", home, mntDir)
	fmt.Printf("\n\nCAUTION:\nwrite operations on %s will also affect $HOME (%s)\n\n", mntDir, home)
	fmt.Printf("Unmount by calling 'fusermount -u %s'\n", mntDir)

	// Serve the file system, until unmounted by calling fusermount -u
	server.Wait()
}
