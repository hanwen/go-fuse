package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/examplelib"
	"fmt"
	"os"
	"flag"
)

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	threaded := flag.Bool("threaded", true, "switch off threading; print debugging messages.")
	flag.Parse()
        if flag.NArg() < 2 {
		// TODO - where to get program name? 
		fmt.Println("usage: main ORIGINAL MOUNTPOINT")
		os.Exit(2)
	}
	
	orig := flag.Arg(0)
	pt := examplelib.NewPassThroughFuse(orig)
	fs := fuse.NewPathFileSystemConnector(pt)
	state := fuse.NewMountState(fs)
	state.Debug = *debug

	mountPoint := flag.Arg(1)
	state.Mount(mountPoint, *threaded)

	fmt.Printf("Mounted %s on %s (threaded=%v, debug=%v)\n", orig, mountPoint, *threaded, *debug)
	
}

