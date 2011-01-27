package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/examplelib"
	"fmt"
	"os"
	"flag"
	"log"
)

var _ = log.Printf

func main() {
	// Scans the arg list and sets up flags
	flag.Parse()
	if flag.NArg() < 2 {
		// TODO - where to get program name?
		fmt.Println("usage: main ZIPFILE MOUNTPOINT")
		os.Exit(2)
	}

	orig := flag.Arg(0)
	fs := examplelib.NewZipFileFuse(orig)
	conn := fuse.NewPathFileSystemConnector(fs)
	state := fuse.NewMountState(conn)

	mountPoint := flag.Arg(1)
	state.Debug = true
	state.Mount(mountPoint)

	fmt.Printf("Mounted %s on %s\n", orig, mountPoint)
	state.Loop(true)
	fmt.Println(state.Stats())
}
