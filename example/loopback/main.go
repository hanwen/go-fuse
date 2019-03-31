// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Mounts another directory as loopback for testing and benchmarking
// purposes.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/nodefs"
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
	// TODO
	// link = flag.Bool("l", false, "Enable hard link support")
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
		log.Printf("send SIGUSR1 to %d to dump memory profile", os.Getpid())
		profSig := make(chan os.Signal, 1)
		signal.Notify(profSig, syscall.SIGUSR1)
		go writeMemProfile(*memprofile, profSig)
	}
	if *cpuprofile != "" || *memprofile != "" {
		fmt.Printf("Note: You must unmount gracefully, otherwise the profile file(s) will stay empty!\n")
	}

	orig, _ := filepath.Abs(flag.Arg(1))
	mountPoint, _ := filepath.Abs(flag.Arg(0))

	loopbackRoot, err := nodefs.NewLoopbackRoot(orig)
	if err != nil {
		log.Fatalf("NewLoopbackRoot: %v", err)
	}

	sec := time.Second
	rawFS := nodefs.NewNodeFS(loopbackRoot, &nodefs.Options{
		EntryTimeout: &sec,
		AttrTimeout:  &sec,
	})

	server, err := fuse.NewServer(rawFS, mountPoint,
		&fuse.MountOptions{
			AllowOther: *other,
			Debug:      *debug,
		})
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err := server.WaitMount()
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Mounted!")
		}
	}()

	server.Serve()
}
