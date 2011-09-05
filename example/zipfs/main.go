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
	debug := flag.Bool("debug", false, "print debugging messages.")
	latencies := flag.Bool("latencies", false, "record operation latencies.")

	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s MOUNTPOINT ZIP-FILE\n", os.Args[0])
		os.Exit(2)
	}

	var fs fuse.FileSystem
	fs, err := zipfs.NewArchiveFileSystem(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewArchiveFileSystem failed: %v\n", err)
		os.Exit(1)
	}
	debugFs := fuse.NewFileSystemDebug()

	if *latencies {
		debugFs.FileSystem = fs
		fs = debugFs
	}

	state, _, err := fuse.MountFileSystem(flag.Arg(0), fs, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	if *latencies {
		debugFs.AddMountState(state)
	}

	state.SetRecordStatistics(*latencies)
	state.Debug = *debug
	state.Loop(true)
}
