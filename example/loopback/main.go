// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is main program driver for the loopback filesystem from
// github.com/hanwen/go-fuse/fs/, a filesystem that shunts operations
// to an underlying file system.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
)

func writeMemProfile(fn string, sigs <-chan os.Signal) {
	i := 0
	for range sigs {
		fn := fmt.Sprintf("%s-%d.memprof", fn, i)
		i++

		log.Printf("Writing mem profile to %s\n", fn)
		f, err := os.Create(fn)
		if err != nil {
			log.Printf("Create: %v", err)
			continue
		}
		pprof.WriteHeapProfile(f)
		if err := f.Close(); err != nil {
			log.Printf("close %v", err)
		}
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	other := flag.Bool("allow-other", false, "mount with -o allowother.")
	quiet := flag.Bool("q", false, "quiet")
	ro := flag.Bool("ro", false, "mount read-only")
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
		if !*quiet {
			fmt.Printf("Writing cpu profile to %s\n", *cpuprofile)
		}
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Println(err)
			os.Exit(3)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if *memprofile != "" {
		if !*quiet {
			log.Printf("send SIGUSR1 to %d to dump memory profile", os.Getpid())
		}
		profSig := make(chan os.Signal, 1)
		signal.Notify(profSig, syscall.SIGUSR1)
		go writeMemProfile(*memprofile, profSig)
	}
	if *cpuprofile != "" || *memprofile != "" {
		if !*quiet {
			fmt.Printf("Note: You must unmount gracefully, otherwise the profile file(s) will stay empty!\n")
		}
	}

	orig := flag.Arg(1)
	loopbackRoot, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		log.Fatalf("NewLoopbackRoot(%s): %v\n", orig, err)
	}

	sec := time.Second
	opts := &fs.Options{
		// These options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		AttrTimeout:  &sec,
		EntryTimeout: &sec,
	}
	opts.Debug = *debug
	opts.AllowOther = *other
	if opts.AllowOther {
		// Make the kernel check file permissions for us
		opts.MountOptions.Options = append(opts.MountOptions.Options, "default_permissions")
	}
	if *ro {
		opts.MountOptions.Options = append(opts.MountOptions.Options, "ro")
	}
	// First column in "df -T": original dir
	opts.MountOptions.Options = append(opts.MountOptions.Options, "fsname="+orig)
	// Second column in "df -T" will be shown as "fuse." + Name
	opts.MountOptions.Name = "loopback"
	// Leave file permissions on "000" files as-is
	opts.NullPermissions = true
	// Enable diagnostics logging
	if !*quiet {
		opts.Logger = log.New(os.Stderr, "", 0)
	}
	server, err := fs.Mount(flag.Arg(0), loopbackRoot, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	if !*quiet {
		fmt.Println("Mounted!")
	}
	server.Wait()
}
