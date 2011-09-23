package main

import (
	"flag"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/unionfs"
	"os"
)

func main() {
	version := flag.Bool("version", false, "print version number")
	debug := flag.Bool("debug", false, "debug on")
	hardlinks := flag.Bool("hardlinks", false, "support hardlinks")
	delcache_ttl := flag.Float64("deletion_cache_ttl", 5.0, "Deletion cache TTL in seconds.")
	branchcache_ttl := flag.Float64("branchcache_ttl", 5.0, "Branch cache TTL in seconds.")
	deldirname := flag.String(
		"deletion_dirname", "GOUNIONFS_DELETIONS", "Directory name to use for deletions.")
	flag.Parse()

	if *version {
		fmt.Println(fuse.Version())
		os.Exit(0)
	}

	if len(flag.Args()) < 2 {
		fmt.Println("Usage:\n  main MOUNTPOINT BASEDIR")
		os.Exit(2)
	}
	ufsOptions := unionfs.UnionFsOptions{
		DeletionCacheTTLSecs: *delcache_ttl,
		BranchCacheTTLSecs:   *branchcache_ttl,
		DeletionDirName:      *deldirname,
	}
	options := unionfs.AutoUnionFsOptions{
		UnionFsOptions: ufsOptions,
		FileSystemOptions: fuse.FileSystemOptions{
			EntryTimeout:    1.0,
			AttrTimeout:     1.0,
			NegativeTimeout: 1.0,
			Owner:           fuse.CurrentOwner(),
		},
		UpdateOnMount: true,
		PathNodeFsOptions: fuse.PathNodeFsOptions{
			ClientInodes: *hardlinks,
		},
	}

	fmt.Printf("AutoUnionFs - Go-FUSE Version %v.\n", fuse.Version())
	gofs := unionfs.NewAutoUnionFs(flag.Arg(1), options)
	pathfs := fuse.NewPathNodeFs(gofs, nil)
	state, conn, err := fuse.MountNodeFileSystem(flag.Arg(0), pathfs, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	pathfs.Debug = *debug
	conn.Debug = *debug
	state.Debug = *debug
	state.Loop()
}
