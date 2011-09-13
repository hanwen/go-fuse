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
	debug := flag.Bool("debug", false, "debug on")
	flag.Parse()
	if flag.NArg() < 1 {
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT")
		os.Exit(2)
	}

	fs := zipfs.NewMultiZipFs()
	state, _, err := fuse.MountPathFileSystem(flag.Arg(0), fs, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	state.Debug = *debug
	state.Loop()
}
