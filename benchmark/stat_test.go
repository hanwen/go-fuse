package fuse

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"github.com/hanwen/go-fuse/fuse"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var CheckSuccess = fuse.CheckSuccess

type StatFs struct {
	fuse.DefaultFileSystem
	entries map[string]*os.FileInfo
	dirs    map[string][]fuse.DirEntry
}

func (me *StatFs) add(name string, fi os.FileInfo) {
	name = strings.TrimRight(name, "/")
	_, ok := me.entries[name]
	if ok {
		return
	}

	me.entries[name] = &fi
	if name == "/" || name == "" {
		return
	}

	dir, base := filepath.Split(name)
	dir = strings.TrimRight(dir, "/")
	me.dirs[dir] = append(me.dirs[dir], fuse.DirEntry{Name: base, Mode: fi.Mode})
	me.add(dir, os.FileInfo{Mode: fuse.S_IFDIR | 0755})
}

func (me *StatFs) GetAttr(name string) (*os.FileInfo, fuse.Status) {
	e := me.entries[name]
	if e == nil {
		return nil, fuse.ENOENT
	}
	return e, fuse.OK
}

func (me *StatFs) OpenDir(name string) (stream chan fuse.DirEntry, status fuse.Status) {
	log.Printf("OPENDIR '%v', %v %v", name, me.entries, me.dirs)
	entries := me.dirs[name]
	if entries == nil {
		return nil, fuse.ENOENT
	}
	stream = make(chan fuse.DirEntry, len(entries))
	for _, e := range entries {
		stream <- e
	}
	close(stream)
	return stream, fuse.OK
}

func NewStatFs() *StatFs {
	return &StatFs{
		entries: make(map[string]*os.FileInfo),
		dirs:    make(map[string][]fuse.DirEntry),
	}
}

func setupFs(fs fuse.FileSystem, opts *fuse.FileSystemOptions) (string, func()) {
	mountPoint := fuse.MakeTempDir()
	state, _, err := fuse.MountFileSystem(mountPoint, fs, opts)
	if err != nil {
		panic(fmt.Sprintf("cannot mount %v", err)) // ugh - benchmark has no error methods.
	}
	// state.Debug = true
	go state.Loop(false)

	return mountPoint, func() {
		err := state.Unmount()
		if err != nil {
			log.Println("error during unmount", err)
		} else {
			os.RemoveAll(mountPoint)
		}
	}
}

func TestNewStatFs(t *testing.T) {
	fs := NewStatFs()
	for _, n := range []string{
		"file.txt", "sub/dir/foo.txt",
		"sub/dir/bar.txt", "sub/marine.txt"} {
		fs.add(n, os.FileInfo{Mode: fuse.S_IFREG | 0644})
	}

	wd, clean := setupFs(fs, nil)
	defer clean()

	names, err := ioutil.ReadDir(wd)
	CheckSuccess(err)
	if len(names) != 2 {
		t.Error("readdir /", names)
	}

	fi, err := os.Lstat(wd + "/sub")
	CheckSuccess(err)
	if !fi.IsDirectory() {
		t.Error("mode", fi)
	}
	names, err = ioutil.ReadDir(wd + "/sub")
	CheckSuccess(err)
	if len(names) != 2 {
		t.Error("readdir /sub", names)
	}
	names, err = ioutil.ReadDir(wd + "/sub/dir")
	CheckSuccess(err)
	if len(names) != 2 {
		t.Error("readdir /sub/dir", names)
	}

	fi, err = os.Lstat(wd + "/sub/marine.txt")
	CheckSuccess(err)
	if !fi.IsRegular() {
		t.Error("mode", fi)
	}
}

func BenchmarkThreadedStat(b *testing.B) {
	b.StopTimer()
	fs := NewStatFs()
	wd, _ := os.Getwd()
	// Names from OpenJDK 1.6
	f, err := os.Open(wd + "/testpaths.txt")
	CheckSuccess(err)

	defer f.Close()
	r := bufio.NewReader(f)

	files := []string{}
	for {
		line, _, err := r.ReadLine()
		if line == nil || err != nil {
			break
		}

		fn := string(line)
		files = append(files, fn)

		fs.add(fn, os.FileInfo{Mode: fuse.S_IFREG | 0644})
	}
	log.Printf("Read %d file names", len(files))

	if len(files) == 0 {
		log.Fatal("no files added")
	}

	ttl := 0.1
	opts := fuse.FileSystemOptions{
		EntryTimeout:    ttl,
		AttrTimeout:     ttl,
		NegativeTimeout: 0.0,
	}
	wd, clean := setupFs(fs, &opts)
	defer clean()

	for i, l := range files {
		files[i] = filepath.Join(wd, l)
	}

	log.Println("N = ", b.N)
	threads := runtime.GOMAXPROCS(0)
	results := TestingBOnePass(b, threads, ttl*1.2, files)
	AnalyzeBenchmarkRuns(results)
}

func TestingBOnePass(b *testing.B, threads int, sleepTime float64, files []string) (results []float64) {
	runs := b.N + 1
	for j := 0; j < runs; j++ {
		if j > 0 {
			b.StartTimer()
		}
		result := BulkStat(threads, files)
		if j > 0 {
			b.StopTimer()
			results = append(results, result)
		} else {
			fmt.Println("Ignoring first run to preheat caches.")
		}

		if j < runs-1 {
			fmt.Printf("Sleeping %.2f seconds\n", sleepTime)
			time.Sleep(int64(sleepTime * 1e9))
		}
	}
	return results
}
