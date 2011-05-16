// Mounts another directory as loopback for testing and benchmarking
// purposes.

package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"fmt"
	"os"
	"flag"
	"runtime"
	"log"
)

var _ = runtime.GOMAXPROCS
var _ = log.Print

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	latencies := flag.Bool("latencies", false, "record latencies.")
	threaded := flag.Bool("threaded", true, "switch off threading; print debugging messages.")
	flag.Parse()
	if flag.NArg() < 2 {
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT ORIGINAL")
		os.Exit(2)
	}

	var finalFs fuse.FileSystem
	orig := flag.Arg(1)
	loopbackfs := fuse.NewLoopbackFileSystem(orig)
	finalFs = loopbackfs

	debugFs := fuse.NewFileSystemDebug()
	if *latencies {
		timing := fuse.NewTimingFileSystem(finalFs)
		debugFs.AddTimingFileSystem(timing)
		finalFs = timing
	}

	opts := &fuse.FileSystemOptions{
		// These options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		NegativeTimeout: 1.0,
		AttrTimeout:     1.0,
		EntryTimeout:    1.0,
	}

	if *latencies {
		debugFs.FileSystem = finalFs
		finalFs = debugFs
	}

	conn := fuse.NewFileSystemConnector(finalFs, opts)
	var finalRawFs fuse.RawFileSystem = conn
	if *latencies {
		rawTiming := fuse.NewTimingRawFileSystem(conn)
		debugFs.AddRawTimingFileSystem(rawTiming)
		finalRawFs = rawTiming
	}

	state := fuse.NewMountState(finalRawFs)
	state.Debug = *debug

	if *latencies {
		state.SetRecordStatistics(true)
		debugFs.AddMountState(state)
		debugFs.AddFileSystemConnector(conn)
	}
	mountPoint := flag.Arg(0)

	fmt.Println("Mounting")
	err := state.Mount(mountPoint, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Mounted!")
	state.Loop(*threaded)
}
