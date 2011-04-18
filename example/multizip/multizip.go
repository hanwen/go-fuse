package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/zipfs"
	"fmt"
	"flag"
	"log"
	"os"
)

var _ = log.Printf

func main() {
	// Scans the arg list and sets up flags
	flag.Parse()
	if flag.NArg() < 1 {
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT")
		os.Exit(2)
	}

	fs := zipfs.NewMultiZipFs()
	state := fuse.NewMountState(fs.Connector)

	mountPoint := flag.Arg(0)
	state.Debug = true
	state.Mount(mountPoint)

	fmt.Printf("Mounted %s\n", mountPoint)
	state.Loop(true)
	fmt.Println(state.Stats())
}
