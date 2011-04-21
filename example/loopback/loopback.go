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
	fuse.WrappingPathFilesystem
}

func (me *PathPrintingFs) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	log.Println(name)
	return me.Original.GetAttr(name)
}

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	threaded := flag.Bool("threaded", true, "switch off threading; print debugging messages.")
	flag.Parse()
	if flag.NArg() < 2 {
		// TODO - where to get program name?
		fmt.Println("usage: main MOUNTPOINT ORIGINAL")
		os.Exit(2)
	}

	var fs fuse.PathFilesystem
	orig := flag.Arg(1)
	loopbackfs := fuse.NewLoopbackFileSystem(orig)
	fs = loopbackfs
	debugFs := new(fuse.PathFilesystemDebug)
	debugFs.Original = fs
	fs = debugFs
	
	timing := fuse.NewTimingPathFilesystem(fs)
	fs = timing

	var opts fuse.PathFileSystemConnectorOptions

	loopbackfs.FillOptions(&opts)

	conn := fuse.NewPathFileSystemConnector(fs)
	debugFs.Connector = conn
	
	rawTiming := fuse.NewTimingRawFilesystem(conn)
	conn.SetOptions(opts)

	state := fuse.NewMountState(rawTiming)
	state.Debug = *debug

	mountPoint := flag.Arg(0)
	err := state.Mount(mountPoint)
	if err != nil {
		fmt.Printf("MountFuse fail: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Mounted %s on %s (threaded=%v, debug=%v)\n", orig, mountPoint, *threaded, *debug)
	state.Loop(*threaded)
	fmt.Println("Finished", state.Stats())

	fmt.Println("\n\nMountState statistics\n")
	counts := state.OperationCounts()
	fmt.Println("Counts: ", counts)

	latency := state.Latencies()
	fmt.Println("Operation latency (ms):")
	fuse.PrintMap(latency)

	latency = rawTiming.Latencies()
	fmt.Println("\n\nRaw FS (ms):", latency)

	fmt.Println("\n\nLoopback FS statistics\n")
	latency = timing.Latencies()
	fmt.Println("Latency (ms):", latency)

	fmt.Println("Operation counts:", timing.OperationCounts())

	hot, unique := timing.HotPaths("GetAttr")
	top := 20
	start := len(hot) - top
	if start < 0 {
		start = 0
	}
	fmt.Printf("Unique GetAttr paths: %d\n", unique)
	fmt.Printf("Top %d GetAttr paths: %v", top, hot[start:])
}
