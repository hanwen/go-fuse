package benchmark

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

var CheckSuccess = fuse.CheckSuccess

func setupFs(fs fuse.FileSystem) (string, func()) {
	opts := &fuse.FileSystemOptions{
		EntryTimeout:    0.0,
		AttrTimeout:     0.0,
		NegativeTimeout: 0.0,
	}
	mountPoint, _ := ioutil.TempDir("", "stat_test")
	nfs := fuse.NewPathNodeFs(fs, nil)
	state, _, err := fuse.MountNodeFileSystem(mountPoint, nfs, opts)
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
		fs.AddFile(n)
	}

	wd, clean := setupFs(fs)
	defer clean()

	names, err := ioutil.ReadDir(wd)
	CheckSuccess(err)
	if len(names) != 2 {
		t.Error("readdir /", names)
	}

	fi, err := os.Lstat(wd + "/sub")
	CheckSuccess(err)
	if !fi.IsDir() {
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
	if fi.Mode()&os.ModeType != 0 {
		t.Error("mode", fi)
	}
}

func BenchmarkGoFuseThreadedStat(b *testing.B) {
	b.StopTimer()
	fs := NewStatFs()
	fs.delay = delay

	wd, _ := os.Getwd()
	files := ReadLines(wd + "/testpaths.txt")
	for _, fn := range files {
		fs.AddFile(fn)
	}

	wd, clean := setupFs(fs)
	defer clean()

	for i, l := range files {
		files[i] = filepath.Join(wd, l)
	}

	threads := runtime.GOMAXPROCS(0)
	results := TestingBOnePass(b, threads, files)
	AnalyzeBenchmarkRuns("Go-FUSE", results)
}

func TestingBOnePass(b *testing.B, threads int, files []string) (results []float64) {
	runtime.GC()
	todo := b.N

	for todo > 0 {
		if len(files) > todo {
			files = files[:todo]
		}
		b.StartTimer()
		result := BulkStat(threads, files)
		todo -= len(files)
		b.StopTimer()
		results = append(results, result)
	}
	return results
}

func BenchmarkCFuseThreadedStat(b *testing.B) {
	b.StopTimer()

	wd, _ := os.Getwd()
	lines := ReadLines(wd + "/testpaths.txt")
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
	for k := range unique {
		out = append(out, k)
	}

	f, err := ioutil.TempFile("", "")
	CheckSuccess(err)
	sort.Strings(out)
	for _, k := range out {
		f.Write([]byte(fmt.Sprintf("/%s\n", k)))
	}
	f.Close()

	mountPoint, _ := ioutil.TempDir("", "stat_test")
	cmd := exec.Command(wd+"/cstatfs",
		"-o",
		"entry_timeout=0.0,attr_timeout=0.0,ac_attr_timeout=0.0,negative_timeout=0.0",
		mountPoint)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("STATFS_INPUT=%s", f.Name()),
		fmt.Sprintf("STATFS_DELAY_USEC=%d", delay / time.Microsecond))
	cmd.Start()

	bin, err := exec.LookPath("fusermount")
	CheckSuccess(err)
	stop := exec.Command(bin, "-u", mountPoint)
	CheckSuccess(err)
	defer stop.Run()

	for i, l := range lines {
		lines[i] = filepath.Join(mountPoint, l)
	}

	time.Sleep(10 * time.Millisecond)
	os.Lstat(mountPoint)
	threads := runtime.GOMAXPROCS(0)
	results := TestingBOnePass(b, threads, lines)
	AnalyzeBenchmarkRuns("CFuse", results)
}

