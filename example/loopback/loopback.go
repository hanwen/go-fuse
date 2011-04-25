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

type PathPrintingFs struct {
	fuse.WrappingFileSystem
}

func (me *PathPrintingFs) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	log.Println(name)
	return me.Original.GetAttr(name)
}

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

	var opts fuse.FileSystemConnectorOptions

	loopbackfs.FillOptions(&opts)

	if *latencies {
		debugFs.Original = finalFs
		finalFs = debugFs
	}

	conn := fuse.NewFileSystemConnector(finalFs)
	var finalRawFs fuse.RawFileSystem = conn
	if *latencies {
		rawTiming := fuse.NewTimingRawFileSystem(conn)
		debugFs.AddRawTimingFileSystem(rawTiming)
		finalRawFs = rawTiming
	}
	conn.SetOptions(opts)

	state := fuse.NewMountState(finalRawFs)
	state.Debug = *debug

	if *latencies {
		state.RecordStatistics = true
		debugFs.AddMountState(state)
		debugFs.AddFileSystemConnector(conn)
	}
	mountPoint := flag.Arg(0)
	err := state.Mount(mountPoint)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	state.Loop(*threaded)
}
