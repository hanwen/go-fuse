// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/hanwen/go-fuse/fuse"
)

// An example of creating a loopback file system, and mounting it onto
// a directory
func ExampleMount() {
	mntDir, _ := ioutil.TempDir("", "")

	home := os.Getenv("HOME")
	root, err := NewLoopbackRoot(home)
	if err != nil {
		log.Panic(err)
	}

	server, err := Mount(mntDir, root, &Options{
		MountOptions: fuse.MountOptions{Debug: true},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted %s as loopback on %s", home, mntDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", mntDir)
	server.Wait()
}
