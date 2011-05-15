package main

import (
	"flag"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/unionfs"
	"os"
)

func main() {
	debug := flag.Bool("debug", false, "debug on")
	threaded := flag.Bool("threaded", true, "debug on")
	delcache_ttl := flag.Float64("deletion_cache_ttl", 5.0, "Deletion cache TTL in seconds.")
	branchcache_ttl := flag.Float64("branchcache_ttl", 5.0, "Branch cache TTL in seconds.")
	deldirname := flag.String(
		"deletion_dirname", "GOUNIONFS_DELETIONS", "Directory name to use for deletions.")
	flag.Parse()

	if len(flag.Args()) < 2 {
		fmt.Println("Usage:\n  main MOUNTPOINT RW-DIRECTORY RO-DIRECTORY ...")
		os.Exit(2)
	}

	ufsOptions := unionfs.UnionFsOptions{
		DeletionCacheTTLSecs: *delcache_ttl,
		BranchCacheTTLSecs:   *branchcache_ttl,
		DeletionDirName:      *deldirname,
	}

	fses := make([]fuse.FileSystem, 0)
	for _, r := range flag.Args()[1:] {
		fses = append(fses, fuse.NewLoopbackFileSystem(r))
	}
	
	ufs := unionfs.NewUnionFs("unionfs", fses, ufsOptions)
	mountState, _, err := fuse.MountFileSystem(flag.Arg(0), ufs, nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(1)
	}

	mountState.Debug = *debug
	mountState.Loop(*threaded)
}
