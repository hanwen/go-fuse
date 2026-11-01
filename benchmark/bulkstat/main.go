// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

func BulkStat(parallelism int, files []string) {
	todo := make(chan string, len(files))
	var wg sync.WaitGroup
	wg.Add(parallelism)
	for i := 0; i < parallelism; i++ {
		go func() {
			for {
				fn := <-todo
				if fn == "" {
					break
				}
				_, err := os.Lstat(fn)
				if err != nil {
					log.Fatal("All stats should succeed:", err)
				}
			}
			wg.Done()
		}()
	}

	for _, v := range files {
		todo <- v
	}
	close(todo)
	wg.Wait()
}

func ReadLines(name string) []string {
	f, err := os.Open(name)
	if err != nil {
		log.Fatal("ReadLines: ", err)
	}
	defer f.Close()
	r := bufio.NewReader(f)

	l := []string{}
	for {
		line, _, err := r.ReadLine()
		if line == nil || err != nil {
			break
		}

		fn := string(line)
		l = append(l, fn)
	}
	if len(l) == 0 {
		log.Fatal("no files added")
	}

	return l
}

// runParallel calls op n times in total, spread over parallelism
// goroutines.
func runParallel(parallelism, n int, op func(buf []byte) error) {
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		count := n / parallelism
		if i < n%parallelism {
			count++
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 32768)
			for j := 0; j < count; j++ {
				if err := op(buf); err != nil {
					log.Fatal(err)
				}
			}
		}()
	}
	wg.Wait()
}

// negStat stats a nonexistent file.
func negStat(fn string) func([]byte) error {
	return func([]byte) error {
		var st syscall.Stat_t
		if err := syscall.Stat(fn, &st); err != syscall.ENOENT {
			return fmt.Errorf("stat %s: got %v, want ENOENT", fn, err)
		}
		return nil
	}
}

// readDir reads the full contents of directory dir.
func readDir(dir string) func([]byte) error {
	return func(buf []byte) error {
		fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer syscall.Close(fd)
		for {
			n, err := syscall.ReadDirent(fd, buf)
			if err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
		}
	}
}

func main() {
	N := flag.Int("N", 1000, "how many files to stat")
	cpu := flag.Int("cpu", 1, "how many threads to use")
	prefix := flag.String("prefix", "", "mount point")
	quiet := flag.Bool("quiet", false, "be quiet")
	mode := flag.String("mode", "stat", "stat: lstat the files listed in the argument; "+
		"negstat: stat nonexistent file in prefix; readdir: read directory prefix")
	flag.Parse()

	switch *mode {
	case "stat":
	case "negstat":
		runParallel(*cpu, *N, negStat(filepath.Join(*prefix, "nonexistent")))
		return
	case "readdir":
		runParallel(*cpu, *N, readDir(*prefix))
		return
	default:
		log.Fatalf("unknown mode %q", *mode)
	}

	f := flag.Arg(0)
	files := ReadLines(f)
	for i, f := range files {
		files[i] = filepath.Join(*prefix, f)
	}
	if !*quiet {
		log.Printf("statting %d with %d threads; first file %s (%d names)", *N, *cpu, files[0], len(files))
	}
	todo := *N
	for todo > 0 {
		if len(files) > todo {
			files = files[:todo]
		}
		BulkStat(*cpu, files)
		todo -= len(files)
	}
}
