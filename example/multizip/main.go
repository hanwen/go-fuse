package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/hanwen/go-fuse/zipfs"
)

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
	state, _, err := nodefs.MountRoot(flag.Arg(0), nfs.Root(), nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	state.SetDebug(*debug)
	state.Serve()
}
