package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/examplelib"
	"fmt"
	"os"
	"expvar"
	"flag"
	"runtime"
	"strings"
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
	fs := examplelib.NewPassThroughFuse(orig)
	conn := fuse.NewPathFileSystemConnector(fs)
	state := fuse.NewMountState(conn)
	state.Debug = *debug

	mountPoint := flag.Arg(1)
	state.Mount(mountPoint)
	cpus := fuse.CountCpus()
	if cpus > 1 {
		runtime.GOMAXPROCS(cpus)
	}

	fmt.Printf("Mounted %s on %s (threaded=%v, debug=%v, cpus=%v)\n", orig, mountPoint, *threaded, *debug, cpus)
	state.Loop(*threaded)
	fmt.Println("Finished", state.Stats())

	for v := range expvar.Iter() {
		if strings.HasPrefix(v.Key, "mount") {
			fmt.Printf("%v: %v\n", v.Key, v.Value)
		}
	}
}
