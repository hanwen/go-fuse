// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// benchguest runs inside the benchmark VM. It mounts a file system,
// runs one workload against it and prints the timings on stdout, which
// is the VM's serial console.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type entry struct {
	path string
	size int64
}

type counters struct {
	files atomic.Int64
	bytes atomic.Int64
}

func main() {
	fstype := flag.String("fstype", "virtiofs", "file system type")
	source := flag.String("source", "myfs", "mount source")
	mnt := flag.String("mnt", "/mnt", "mount point")
	workload := flag.String("workload", "", "stat, readall, subset, randread")
	list := flag.String("list", "/list.txt", "file with 'size path' lines for subset and randread")
	par := flag.Int("par", 1, "parallel workers")
	reads := flag.Int("reads", 2000, "number of reads for randread")
	ioSize := flag.Int("iosize", 4096, "read size for randread")
	flag.Parse()

	if err := run(*fstype, *source, *mnt, *workload, *list, *par, *reads, *ioSize); err != nil {
		fmt.Printf("BENCH-ERROR %v\n", err)
	}
	fmt.Println("BENCH-DONE")
}

func run(fstype, source, mnt, workload, list string, par, reads, ioSize int) error {
	var entries []entry
	if workload == "subset" || workload == "randread" {
		var err error
		if entries, err = readList(list); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(mnt, 0755); err != nil {
		return err
	}
	start := time.Now()
	if err := syscall.Mount(source, mnt, fstype, syscall.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("mount %s: %v", fstype, err)
	}
	mountDur := time.Since(start)

	var c counters
	start = time.Now()
	var err error
	switch workload {
	case "stat":
		err = walk(mnt, &c, false)
	case "readall":
		err = walk(mnt, &c, true)
	case "subset":
		err = readSubset(mnt, entries, par, &c)
	case "randread":
		err = randRead(mnt, entries, par, reads, ioSize, &c)
	default:
		err = fmt.Errorf("unknown workload %q", workload)
	}
	workDur := time.Since(start)
	if err != nil {
		return err
	}
	fmt.Printf("BENCH mount_ns=%d work_ns=%d files=%d bytes=%d\n",
		mountDur.Nanoseconds(), workDur.Nanoseconds(), c.files.Load(), c.bytes.Load())
	return nil
}

func readList(name string) ([]entry, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var res []entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sz, p, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(sz, 10, 64)
		if err != nil {
			return nil, err
		}
		res = append(res, entry{p, n})
	}
	return res, sc.Err()
}

var bufPool = sync.Pool{New: func() any { b := make([]byte, 1<<20); return &b }}

func readFile(name string, c *counters) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufPool.Get().(*[]byte)
	defer bufPool.Put(buf)
	for {
		n, err := f.Read(*buf)
		c.bytes.Add(int64(n))
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	c.files.Add(1)
	return nil
}

func walk(root string, c *counters, read bool) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var st syscall.Stat_t
		if err := syscall.Lstat(p, &st); err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !read {
			c.files.Add(1)
			return nil
		}
		return readFile(p, c)
	})
}

func parallel(n, par int, fn func(i int) error) error {
	var next atomic.Int64
	var wg sync.WaitGroup
	errs := make([]error, par)
	for w := range par {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				if err := fn(i); err != nil {
					errs[w] = err
					return
				}
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func readSubset(mnt string, entries []entry, par int, c *counters) error {
	return parallel(len(entries), par, func(i int) error {
		return readFile(filepath.Join(mnt, entries[i].path), c)
	})
}

// randRead does random aligned preads across the listed files, like a
// database or an index lookup.
func randRead(mnt string, entries []entry, par, reads, ioSize int, c *counters) error {
	var cands []entry
	for _, e := range entries {
		if e.size >= int64(ioSize) {
			cands = append(cands, e)
		}
	}
	if len(cands) == 0 {
		return fmt.Errorf("no files of size >= %d", ioSize)
	}

	type op struct {
		idx int
		off int64
	}
	rng := rand.New(rand.NewPCG(1, 2))
	ops := make([]op, reads)
	for i := range ops {
		e := rng.IntN(len(cands))
		ops[i] = op{e, rng.Int64N(cands[e].size/int64(ioSize)) * int64(ioSize)}
	}

	var mu sync.Mutex
	files := map[int]*os.File{}
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	open := func(idx int) (*os.File, error) {
		mu.Lock()
		defer mu.Unlock()
		if f := files[idx]; f != nil {
			return f, nil
		}
		f, err := os.Open(filepath.Join(mnt, cands[idx].path))
		if err != nil {
			return nil, err
		}
		files[idx] = f
		c.files.Add(1)
		return f, nil
	}

	return parallel(len(ops), par, func(i int) error {
		f, err := open(ops[i].idx)
		if err != nil {
			return err
		}
		buf := make([]byte, ioSize)
		n, err := f.ReadAt(buf, ops[i].off)
		c.bytes.Add(int64(n))
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	})
}
