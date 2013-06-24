// Mounts MemNodeFs for testing purposes.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	flag.Parse()
	if flag.NArg() < 2 {
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT BACKING-PREFIX")
		os.Exit(2)
	}

	mountPoint := flag.Arg(0)
	prefix := flag.Arg(1)
	fs := nodefs.NewMemNodeFs(prefix)
	conn := nodefs.NewFileSystemConnector(fs, nil)
	state := fuse.NewMountState(conn.RawFS())
	state.SetDebug(*debug)

	fmt.Println("Mounting")
	err := state.Mount(mountPoint, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Mounted!")
	state.Loop()
}
