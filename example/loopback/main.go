// Mounts another directory as loopback for testing and benchmarking
// purposes.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

var _ = runtime.GOMAXPROCS
var _ = log.Print

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

	opts := &fuse.FileSystemOptions{
		// These options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		NegativeTimeout: time.Second,
		AttrTimeout:     time.Second,
		EntryTimeout:    time.Second,
	}
	pathFs := pathfs.NewPathNodeFs(finalFs, nil)
	conn := fuse.NewFileSystemConnector(pathFs, opts)
	state := fuse.NewMountState(conn.RawFS())
	state.SetDebug(*debug)

	mountPoint := flag.Arg(0)

	fmt.Println("Mounting")
	mOpts := &fuse.MountOptions{
		AllowOther: *other,
	}
	err := state.Mount(mountPoint, mOpts)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Mounted!")
	state.Loop()
}
