// Mounts another directory as loopback for testing and benchmarking
// purposes.

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	other := flag.Bool("allow-other", false, "mount with -o allowother.")
	flag.Parse()
	if flag.NArg() < 2 {
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT ORIGINAL")
		os.Exit(2)
	}

	var finalFs pathfs.FileSystem
	orig := flag.Arg(1)
	loopbackfs := pathfs.NewLoopbackFileSystem(orig)
	finalFs = loopbackfs

	opts := &nodefs.Options{
		// These options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		NegativeTimeout: time.Second,
		AttrTimeout:     time.Second,
		EntryTimeout:    time.Second,
	}
	pathFs := pathfs.NewPathNodeFs(finalFs, nil)
	conn := nodefs.NewFileSystemConnector(pathFs.Root(), opts)
	mountPoint := flag.Arg(0)
	mOpts := &fuse.MountOptions{
		AllowOther: *other,
	}
	state, err := fuse.NewServer(conn.RawFS(), mountPoint, mOpts)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}
	state.SetDebug(*debug)

	fmt.Println("Mounted!")
	state.Serve()
}
