// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// statfs is a main driver for the file system from
// github.com/hanwen/go-fuse/benchmark, intended for benchmarking FUSE
// libraries.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/benchmark"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func main() {
	// Scans the arg list and sets up flags
	debug := flag.Bool("debug", false, "print debugging messages.")
	profile := flag.String("profile", "", "record cpu profile.")
	mem_profile := flag.String("mem-profile", "", "record memory profile.")
	command := flag.String("run", "", "run this command after mounting.")
	ttl := flag.Duration("ttl", time.Second, "attribute/entry cache TTL.")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s MOUNTPOINT FILENAMES-FILE\n", os.Args[0])
		os.Exit(2)
	}

	var profFile, memProfFile io.Writer
	var err error
	if *profile != "" {
		profFile, err = os.Create(*profile)
		if err != nil {
			log.Fatalf("os.Create: %v", err)
		}
	}
	if *mem_profile != "" {
		memProfFile, err = os.Create(*mem_profile)
		if err != nil {
			log.Fatalf("os.Create: %v", err)
		}
	}

	root := &benchmark.StatFS{}
	lines := benchmark.ReadLines(flag.Arg(1))
	for _, l := range lines {
		root.AddFile(strings.TrimSpace(l),
			fuse.Attr{Mode: syscall.S_IFREG})
	}
	opts := &fs.Options{
		AttrTimeout:  ttl,
		EntryTimeout: ttl,
	}
	opts.Debug = *debug
	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	runtime.GC()
	if profFile != nil {
		pprof.StartCPUProfile(profFile)
		defer pprof.StopCPUProfile()
	}

	if *command != "" {
		args := strings.Split(*command, " ")
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Start()
	}

	server.Wait()
	if memProfFile != nil {
		pprof.WriteHeapProfile(memProfFile)
	}
}
