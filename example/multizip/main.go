package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/hanwen/go-fuse/zipfs"
)

var _ = log.Printf

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "debug on")
	flag.Parse()
	if flag.NArg() < 1 {
		_, prog := filepath.Split(os.Args[0])
		fmt.Printf("usage: %s MOUNTPOINT\n", prog)
		os.Exit(2)
	}

	fs := zipfs.NewMultiZipFs()
	nfs := pathfs.NewPathNodeFs(fs, nil)
	state, _, err := fuse.MountNodeFileSystem(flag.Arg(0), nfs, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	state.Debug = *debug
	state.Loop()
}
