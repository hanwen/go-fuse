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
	"strings"
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

// statFiles has goroutine i of parallelism stat prefix/file<i>, n
// times in total.
func statFiles(parallelism, n int, prefix string) {
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		count := n / parallelism
		if i < n%parallelism {
			count++
		}
		fn := filepath.Join(prefix, fmt.Sprintf("file%d", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			var st syscall.Stat_t
			for j := 0; j < count; j++ {
				if err := syscall.Stat(fn, &st); err != nil {
					log.Fatalf("stat %s: %v", fn, err)
				}
			}
		}()
	}
	wg.Wait()
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

// runIO has each of parallelism goroutines read or write n blocks of
// size bs sequentially from the start of name. If name contains %d,
// it is replaced by the goroutine index.
func runIO(parallelism, n, bs int, name string, write, direct bool) {
	flags := syscall.O_RDONLY
	if write {
		flags = syscall.O_WRONLY | syscall.O_CREAT
	}
	if direct {
		flags |= oDirect
	}
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		fn := name
		if strings.Contains(fn, "%d") {
			fn = fmt.Sprintf(fn, i)
		}
		// O_DIRECT needs an aligned buffer; mmap returns page-aligned memory.
		buf, err := syscall.Mmap(-1, 0, bs, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
		if err != nil {
			log.Fatalf("mmap: %v", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			fd, err := syscall.Open(fn, flags|syscall.O_CLOEXEC, 0644)
			if err != nil {
				log.Fatalf("open %s: %v", fn, err)
			}
			defer syscall.Close(fd)
			for j := 0; j < n; j++ {
				var m int
				if write {
					m, err = syscall.Pwrite(fd, buf, int64(j)*int64(bs))
				} else {
					m, err = syscall.Pread(fd, buf, int64(j)*int64(bs))
				}
				if err != nil {
					log.Fatalf("%s: %v", fn, err)
				}
				if m != bs {
					log.Fatalf("%s: block %d: got %d bytes, want %d", fn, j, m, bs)
				}
			}
		}()
	}
	wg.Wait()
}

func main() {
	N := flag.Int("N", 1000, "number of operations (for read and write: blocks per thread)")
	cpu := flag.Int("cpu", 1, "how many threads to use")
	prefix := flag.String("prefix", "", "mount point")
	quiet := flag.Bool("quiet", false, "be quiet")
	mode := flag.String("mode", "stat", "stat: lstat the files listed in the argument; "+
		"negstat: stat nonexistent file in prefix; getattr: stat prefix/file<thread>; readdir: read directory prefix; "+
		"read, write: read or write N blocks per thread of the file named by the argument")
	bs := flag.Int("bs", 128*1024, "block size for read and write")
	direct := flag.Bool("direct", false, "use O_DIRECT for read and write")
	flag.Parse()

	switch *mode {
	case "stat":
	case "negstat":
		runParallel(*cpu, *N, negStat(filepath.Join(*prefix, "nonexistent")))
		return
	case "getattr":
		statFiles(*cpu, *N, *prefix)
		return
	case "readdir":
		runParallel(*cpu, *N, readDir(*prefix))
		return
	case "read", "write":
		runIO(*cpu, *N, *bs, filepath.Join(*prefix, flag.Arg(0)), *mode == "write", *direct)
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
