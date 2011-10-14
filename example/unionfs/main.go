package main

import (
	"flag"
	"fmt"
	"log"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/unionfs"
	"os"
)

func main() {
	debug := flag.Bool("debug", false, "debug on")
	mem := flag.Bool("mem", false, "use in-memory unionfs")
	portable := flag.Bool("portable", false, "use 32 bit inodes")

	entry_ttl := flag.Float64("entry_ttl", 1.0, "fuse entry cache TTL.")
	negative_ttl := flag.Float64("negative_ttl", 1.0, "fuse negative entry cache TTL.")

	delcache_ttl := flag.Float64("deletion_cache_ttl", 5.0, "Deletion cache TTL in seconds.")
	branchcache_ttl := flag.Float64("branchcache_ttl", 5.0, "Branch cache TTL in seconds.")
	deldirname := flag.String(
		"deletion_dirname", "GOUNIONFS_DELETIONS", "Directory name to use for deletions.")
	flag.Parse()
	if len(flag.Args()) < 2 {
		fmt.Println("Usage:\n  unionfs MOUNTPOINT RW-DIRECTORY RO-DIRECTORY ...")
		os.Exit(2)
	}

	var nodeFs fuse.NodeFileSystem
	if *mem {
		nodeFs = unionfs.NewMemUnionFs(
			flag.Arg(1)+"/", &fuse.LoopbackFileSystem{Root: flag.Arg(2)})
	} else {
		ufsOptions := unionfs.UnionFsOptions{
			DeletionCacheTTLSecs: *delcache_ttl,
			BranchCacheTTLSecs:   *branchcache_ttl,
			DeletionDirName:      *deldirname,
		}

		ufs, err := unionfs.NewUnionFsFromRoots(flag.Args()[1:], &ufsOptions, true)
		if err != nil {
			log.Fatal("Cannot create UnionFs", err)
			os.Exit(1)
		}
		nodeFs = fuse.NewPathNodeFs(ufs, &fuse.PathNodeFsOptions{ClientInodes: true})
	}
	mOpts := fuse.FileSystemOptions{
		EntryTimeout:    *entry_ttl,
		AttrTimeout:     *entry_ttl,
		NegativeTimeout: *negative_ttl,
		PortableInodes:  *portable,
	}
	mountState, _, err := fuse.MountNodeFileSystem(flag.Arg(0), nodeFs, &mOpts)
	if err != nil {
		log.Fatal("Mount fail:", err)
	}

	mountState.Debug = *debug
	mountState.Loop()
}
