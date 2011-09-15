package fuse

import (
	"bufio"
	"exec"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

func (me *StatFs) GetAttr(name string, context *fuse.Context) (*os.FileInfo, fuse.Status) {
	e := me.entries[name]
	if e == nil {
		return nil, fuse.ENOENT
	}
	return e, fuse.OK
}

func (me *StatFs) OpenDir(name string, context *fuse.Context) (stream chan fuse.DirEntry, status fuse.Status) {
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
	mountPoint, _ := ioutil.TempDir("", "stat_test")
	state, _, err := fuse.MountPathFileSystem(mountPoint, fs, opts)
	if err != nil {
		panic(fmt.Sprintf("cannot mount %v", err)) // ugh - benchmark has no error methods.
	}
	// state.Debug = true
	go state.Loop()

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

func GetTestLines() []string {
	wd, _ := os.Getwd()
	// Names from OpenJDK 1.6
	fn := wd + "/testpaths.txt"

	f, err := os.Open(fn)
	CheckSuccess(err)

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
	return l
}

func BenchmarkGoFuseThreadedStat(b *testing.B) {
	b.StopTimer()
	fs := NewStatFs()
	files := GetTestLines()
	for _, fn := range files {
		fs.add(fn, os.FileInfo{Mode: fuse.S_IFREG | 0644})
	}
	if len(files) == 0 {
		log.Fatal("no files added")
	}

	log.Printf("Read %d file names", len(files))

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

func BenchmarkCFuseThreadedStat(b *testing.B) {
	log.Println("benchmarking CFuse")

	lines := GetTestLines()
	unique := map[string]int{}
	for _, l := range lines {
		unique[l] = 1
		dir, _ := filepath.Split(l)
		for dir != "/" && dir != "" {
			unique[dir] = 1
			dir = filepath.Clean(dir)
			dir, _ = filepath.Split(dir)
		}
	}

	out := []string{}
	for k, _ := range unique {
		out = append(out, k)
	}

	f, err := ioutil.TempFile("", "")
	CheckSuccess(err)
	sort.Strings(out)
	for _, k := range out {
		f.Write([]byte(fmt.Sprintf("/%s\n", k)))
	}
	f.Close()

	log.Println("Written:", f.Name())
	mountPoint, _ := ioutil.TempDir("", "stat_test")
	wd, _ := os.Getwd()
	cmd := exec.Command(wd+"/cstatfs", mountPoint)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STATFS_INPUT=%s", f.Name()))
	cmd.Start()

	bin, err := exec.LookPath("fusermount")
	CheckSuccess(err)
	stop := exec.Command(bin, "-u", mountPoint)
	CheckSuccess(err)
	defer stop.Run()

	for i, l := range lines {
		lines[i] = filepath.Join(mountPoint, l)
	}

	// Wait for the daemon to mount.
	time.Sleep(0.2e9)
	ttl := 1.0
	log.Println("N = ", b.N)
	threads := runtime.GOMAXPROCS(0)
	results := TestingBOnePass(b, threads, ttl*1.2, lines)
	AnalyzeBenchmarkRuns(results)
}
