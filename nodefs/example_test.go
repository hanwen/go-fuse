// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/nodefs"
)

// mountLoopback mounts dir under the given mountpoint
func mountLoopback(dir, mntPoint string) (*fuse.Server, error) {
	root, err := nodefs.NewLoopbackRoot(dir)
	if err != nil {
		return nil, err
	}

	// Make the root available under mntDir
	return nodefs.Mount(mntPoint, root, &nodefs.Options{
		MountOptions: fuse.MountOptions{Debug: true},
	})
}

// An example of creating a loopback file system, and mounting it onto
// a directory
func Example_mountLoopback() {
	mntDir, _ := ioutil.TempDir("", "")

	home := os.Getenv("HOME")

	// Make $HOME available on a mount dir under /tmp/ . Caution:
	// write operations are also mirrored.
	server, err := mountLoopback(mntDir, home)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("Mounted %s as loopback on %s\n", home, mntDir)
	fmt.Printf("\n\nCAUTION:\nwrite operations on %s will also affect $HOME (%s)\n\n", mntDir, home)
	fmt.Printf("Unmount by calling 'fusermount -u %s'\n", mntDir)

	// Wait until the directory is unmounted
	server.Wait()
}
