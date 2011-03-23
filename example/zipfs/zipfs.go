package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/examplelib"
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
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT ZIP-FILE")
		os.Exit(2)
	}

	fs := examplelib.NewZipArchiveFileSystem(flag.Arg(1))
	conn := fuse.NewPathFileSystemConnector(fs)
	state := fuse.NewMountState(conn)

	mountPoint := flag.Arg(0)
	state.Debug = *debug
	state.Mount(mountPoint)

	fmt.Printf("Mounted %s - PID %s\n", mountPoint, fuse.MyPID())
	state.Loop(true)

	latency := state.Latencies()
	fmt.Println("Operation latency (ms):")
	fuse.PrintMap(latency)

	fmt.Println("Memory stats", state.Stats())
}
