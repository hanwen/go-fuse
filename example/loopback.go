// Mounts another directory as loopback for testing and benchmarking
// purposes.

package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"fmt"
	"os"
	"flag"
	"runtime"
	"sort"
	"log"
)

var _ = runtime.GOMAXPROCS
var _ = log.Print

func PrintMap(m map[string]float64)  {
	keys := make([]string, len(m))
	for k, _ := range m {
		keys = append(keys, k)
	}

	sort.SortStrings(keys)
	for _, k := range keys {
		if m[k] > 0 {
			fmt.Println(k, m[k])
		}
	}
}

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
	fs := fuse.NewLoopbackFileSystem(orig)
	timing := fuse.NewTimingPathFilesystem(fs)

	var opts fuse.PathFileSystemConnectorOptions

	opts.AttrTimeout = 0.0
	opts.EntryTimeout = 0.0
	opts.NegativeTimeout = 0.0

	fs.FillOptions(&opts)

	conn := fuse.NewPathFileSystemConnector(timing)
	rawTiming := fuse.NewTimingRawFilesystem(conn)
	conn.SetOptions(opts)

	state := fuse.NewMountState(rawTiming)
	state.Debug = *debug

	mountPoint := flag.Arg(1)
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
	PrintMap(latency)

	latency = rawTiming.Latencies()
	fmt.Println("\n\nRaw FS (ms):", latency)

	fmt.Println("\n\nLoopback FS statistics\n")
	latency = timing.Latencies()
	fmt.Println("Latency (ms):", latency)

	fmt.Println("Operation counts:", timing.OperationCounts())

	hot, unique := timing.HotPaths("GetAttr")
	top := 200
	start := len(hot)-top
	if start < 0 {
		start = 0
	}
	fmt.Printf("Unique GetAttr paths: %d\n", unique)
	fmt.Printf("Top %d GetAttr paths: %v", top, hot[start:])
}
