// Mounts another directory as loopback for testing and benchmarking
// purposes.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

func main() {
	log.SetFlags(log.Lmicroseconds)
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	other := flag.Bool("allow-other", false, "mount with -o allowother.")
	enableLinks := flag.Bool("l", false, "Enable hard link support")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to this file")
	memprofile := flag.String("memprofile", "", "write memory profile to this file")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Printf("usage: %s MOUNTPOINT ORIGINAL\n", path.Base(os.Args[0]))
		fmt.Printf("\noptions:\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if *cpuprofile != "" {
		fmt.Printf("Writing cpu profile to %s\n", *cpuprofile)
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Println(err)
			os.Exit(3)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if *memprofile != "" {
		fmt.Printf("Writing mem profile to %s\n", *memprofile)
		f, err := os.Create(*memprofile)
		if err != nil {
			fmt.Println(err)
			os.Exit(4)
		}
		defer func() {
			pprof.WriteHeapProfile(f)
			f.Close()
			return
		}()
	}
	if *cpuprofile != "" || *memprofile != "" {
		fmt.Printf("Note: You must unmount gracefully, otherwise the profile file(s) will stay empty!\n")
	}

	var finalFs pathfs.FileSystem
	orig := flag.Arg(1)
	loopbackfs := pathfs.NewLoopbackFileSystem(orig)
	finalFs = loopbackfs

	opts := &nodefs.Options{
		// These options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		NegativeTimeout: time.Second,
		AttrTimeout:     time.Second,
		EntryTimeout:    time.Second,
	}
	// Enable ClientInodes so hard links work
	pathFsOpts := &pathfs.PathNodeFsOptions{ClientInodes: *enableLinks}
	pathFs := pathfs.NewPathNodeFs(finalFs, pathFsOpts)
	conn := nodefs.NewFileSystemConnector(pathFs.Root(), opts)
	mountPoint := flag.Arg(0)
	origAbs, _ := filepath.Abs(orig)
	mOpts := &fuse.MountOptions{
		AllowOther: *other,
		Name:       "loopbackfs",
		FsName:     origAbs,
	}
	state, err := fuse.NewServer(conn.RawFS(), mountPoint, mOpts)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}
	state.SetDebug(*debug)

	fmt.Println("Mounted!")
	state.Serve()
}
