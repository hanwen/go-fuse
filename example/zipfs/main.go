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
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s MOUNTPOINT ZIP-FILE\n", os.Args[0])
		os.Exit(2)
	}

	fs, err := zipfs.NewZipArchiveFileSystem(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewZipArchiveFileSystem failed: %v\n", err)
		os.Exit(1)
	}
	conn := fuse.NewFileSystemConnector(fs)
	state := fuse.NewMountState(conn)

	mountPoint := flag.Arg(0)
	state.Debug = *debug
	state.Mount(mountPoint)

	fmt.Printf("Mounted %s - PID %s\n", mountPoint, fuse.MyPID())
	state.Loop(true)

	latency := state.Latencies()
	fmt.Println("Operation latency (ms):")
	fuse.PrintMap(latency)
	counts := state.OperationCounts()
	fmt.Println("Counts: ", counts)

	fmt.Println("Memory stats", state.Stats())
}
