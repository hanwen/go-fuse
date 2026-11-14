// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	gofs "github.com/hanwen/go-fuse/v3/fs"
)

var (
	vmbenchTree     = flag.String("vmbench.tree", "", "benchmark this directory instead of the synthetic trees")
	vmbenchSrcFiles = flag.Int("vmbench.srcfiles", 10000, "number of files in the synthetic source tree")
	vmbenchBlobs    = flag.Int("vmbench.blobs", 8, "number of files in the synthetic blob tree")
	vmbenchBlobSize = flag.Int64("vmbench.blobsize", 64<<20, "size of each blob")
	vmbenchFraction = flag.Float64("vmbench.fraction", 0.1, "fraction of files read by the subset workloads")
	vmbenchPar      = flag.Int("vmbench.par", 8, "concurrency of the -par workloads")
	vmbenchReads    = flag.Int("vmbench.reads", 2000, "number of reads for randread")
	vmbenchEntryTTL = flag.Duration("vmbench.entryttl", time.Hour, "virtiofs: entry timeout")
	vmbenchAttrTTL  = flag.Duration("vmbench.attrttl", time.Hour, "virtiofs: attribute timeout")
	vmbenchNegTTL   = flag.Duration("vmbench.negttl", time.Hour, "virtiofs: negative entry timeout")
	vmbenchTimeout  = flag.Duration("vmbench.vmtimeout", 10*time.Minute, "timeout for a single VM run")
)

type benchWorkload struct {
	name  string
	guest string
	par   int
	// list is the file list passed to the guest: "subset", "all" or "".
	list string
}

func benchWorkloads() []benchWorkload {
	return []benchWorkload{
		{"stat", "stat", 1, ""},
		{"readall", "readall", 1, ""},
		{"subset", "subset", 1, "subset"},
		{"subset-par", "subset", *vmbenchPar, "subset"},
		{"randread", "randread", 1, "all"},
		{"randread-par", "randread", *vmbenchPar, "all"},
	}
}

type treeEntry struct {
	path string
	size int64
}

func genSrcTree(dir string, n int) error {
	rng := rand.New(rand.NewPCG(1, 1))
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(rng.Uint32())
	}
	for i := range n {
		sub := filepath.Join(dir, fmt.Sprintf("d%02d/d%02d", i/1024, (i/32)%32))
		if err := os.MkdirAll(sub, 0755); err != nil {
			return err
		}
		// Lognormal, median 4 KiB, like files in a source tree.
		sz := min(int(4096*math.Exp(1.5*rng.NormFloat64())), len(data)-1)
		off := rng.IntN(len(data) - sz)
		if err := os.WriteFile(filepath.Join(sub, fmt.Sprintf("f%05d.c", i)), data[off:off+sz], 0644); err != nil {
			return err
		}
	}
	return nil
}

func genBlobTree(dir string, n int, size int64) error {
	rng := rand.New(rand.NewPCG(2, 2))
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(rng.Uint32())
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for i := range n {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("blob%02d.bin", i)))
		if err != nil {
			return err
		}
		for left := size; left > 0; {
			k := min(left, int64(len(data)))
			if _, err := f.Write(data[:k]); err != nil {
				f.Close()
				return err
			}
			left -= k
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func listTree(dir string) ([]treeEntry, error) {
	var res []treeEntry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		res = append(res, treeEntry{rel, fi.Size()})
		return nil
	})
	return res, err
}

// subsetOf returns a random fraction of entries, in random order.
func subsetOf(entries []treeEntry, frac float64) []treeEntry {
	n := max(1, int(frac*float64(len(entries))+0.5))
	rng := rand.New(rand.NewPCG(3, 3))
	res := slices.Clone(entries)
	rng.Shuffle(len(res), func(i, j int) { res[i], res[j] = res[j], res[i] })
	return res[:n]
}

func writeList(name string, entries []treeEntry) error {
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "%d %s\n", e.size, e.path)
	}
	return os.WriteFile(name, []byte(sb.String()), 0644)
}

func buildGuest(b *testing.B) string {
	bin := filepath.Join(b.TempDir(), "benchguest")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/hanwen/go-fuse/v3/virtiofs/internal/benchguest")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("build benchguest: %v\n%s", err, out)
	}
	return bin
}

type vmResult struct {
	mount, work time.Duration
}

type vmConfig struct {
	ramdisk string
	// root returns the file system to serve over virtiofs.
	root func() (gofs.InodeEmbedder, error)
	// args are extra arguments for the VMM.
	args []string
}

// runVM boots a VM, which runs benchguest, and returns the timings it
// reports.
func runVM(cfg *vmConfig) (vmResult, error) {
	var res vmResult
	dir, err := os.MkdirTemp("", "vmbench")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(dir)

	root, err := cfg.root()
	if err != nil {
		return res, err
	}
	opts := &gofs.Options{
		EntryTimeout:    vmbenchEntryTTL,
		AttrTimeout:     vmbenchAttrTTL,
		NegativeTimeout: vmbenchNegTTL,
	}
	sock := filepath.Join(dir, "virtiofs.sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		return res, err
	}
	defer l.Close()
	go serve(l, gofs.NewNodeFS(root, opts), &opts.MountOptions)

	cmd := newVMCmd(sock, cfg.ramdisk)
	cmd.Args = append(cmd.Args, cfg.args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return res, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return res, err
	}
	timer := time.AfterFunc(*vmbenchTimeout, func() { cmd.Process.Kill() })
	defer timer.Stop()

	var console []string
	var found, done bool
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		console = append(console, line)
		if rest, ok := strings.CutPrefix(line, "BENCH "); ok {
			for _, kv := range strings.Fields(rest) {
				k, v, _ := strings.Cut(kv, "=")
				n, _ := strconv.ParseInt(v, 10, 64)
				switch k {
				case "mount_ns":
					res.mount = time.Duration(n)
				case "work_ns":
					res.work = time.Duration(n)
				}
			}
			found = true
		}
		if line == "BENCH-DONE" {
			done = true
			break
		}
	}
	cmd.Process.Kill()
	go func() {
		for sc.Scan() {
		}
	}()
	cmd.Wait()
	if !done || !found {
		tail := console[max(0, len(console)-40):]
		return res, fmt.Errorf("guest did not finish:\n%s", strings.Join(tail, "\n"))
	}
	return res, nil
}

func guestInit(mods []string, args []string) []byte {
	return []byte("#!/bin/sh\n" +
		"mount -t proc none /proc\n" +
		"mount -t sysfs none /sys\n" +
		"mkdir -p /dev && mount -t devtmpfs none /dev\n" +
		moduleInsmodLines(mods) +
		"/benchguest " + strings.Join(args, " ") + "\n" +
		"reboot -n -f\n")
}

type benchTree struct {
	name    string
	dir     string
	entries []treeEntry
}

// benchTrees returns the trees to benchmark, generating the synthetic
// ones in a temporary directory.
func benchTrees(b *testing.B) []benchTree {
	var trees []benchTree
	if *vmbenchTree != "" {
		trees = append(trees, benchTree{name: filepath.Base(*vmbenchTree), dir: *vmbenchTree})
	} else {
		work := b.TempDir()
		src := filepath.Join(work, "src")
		if err := genSrcTree(src, *vmbenchSrcFiles); err != nil {
			b.Fatal(err)
		}
		blobs := filepath.Join(work, "blobs")
		if err := genBlobTree(blobs, *vmbenchBlobs, *vmbenchBlobSize); err != nil {
			b.Fatal(err)
		}
		trees = append(trees, benchTree{name: "src", dir: src}, benchTree{name: "blobs", dir: blobs})
	}
	for i := range trees {
		var err error
		if trees[i].entries, err = listTree(trees[i].dir); err != nil {
			b.Fatal(err)
		}
		sort.Slice(trees[i].entries, func(a, c int) bool { return trees[i].entries[a].path < trees[i].entries[c].path })
	}
	return trees
}

// benchRamdisk builds an initrd that mounts fstype from source, and
// runs workload wl on tree.
func benchRamdisk(b *testing.B, guest string, mods []string, tree *benchTree, wl benchWorkload, fstype, source string) string {
	tmp := b.TempDir()
	extra := map[string]string{"/benchguest": guest}
	args := []string{"-fstype", fstype, "-source", source,
		"-workload", wl.guest, "-par", strconv.Itoa(wl.par), "-reads", strconv.Itoa(*vmbenchReads)}
	if wl.list != "" {
		entries := tree.entries
		if wl.list == "subset" {
			entries = subsetOf(entries, *vmbenchFraction)
		}
		list := filepath.Join(tmp, "list.txt")
		if err := writeList(list, entries); err != nil {
			b.Fatal(err)
		}
		extra["/list.txt"] = list
	}
	ramdisk := filepath.Join(tmp, "initrd.cpio.gz")
	if err := mkinitRam(ramdisk, testAssets.busybox, mods, guestInit(mods, args), extra); err != nil {
		b.Fatal(err)
	}
	return ramdisk
}

func reportMs(b *testing.B, d time.Duration, unit string) {
	b.ReportMetric(float64(d)/float64(time.Millisecond)/float64(b.N), unit)
}

// BenchmarkVirtioFS runs file system workloads in a VM against a
// loopback file system served over virtiofs. Each iteration boots a
// fresh VM, so the guest page cache is cold; the host page cache is
// warm. Run with eg.
//
//	go test -run '^$' -bench VirtioFS -benchtime 1x -count 5 -timeout 0 ./virtiofs
func BenchmarkVirtioFS(b *testing.B) {
	guest := buildGuest(b)
	for _, tree := range benchTrees(b) {
		for _, wl := range benchWorkloads() {
			b.Run(fmt.Sprintf("tree=%s/work=%s", tree.name, wl.name), func(b *testing.B) {
				ramdisk := benchRamdisk(b, guest, testAssets.modules, &tree, wl, "virtiofs", "myfs")
				var sum vmResult
				for range b.N {
					res, err := runVM(&vmConfig{
						ramdisk: ramdisk,
						root:    func() (gofs.InodeEmbedder, error) { return gofs.NewLoopbackRoot(tree.dir) },
					})
					if err != nil {
						b.Fatal(err)
					}
					sum.mount += res.mount
					sum.work += res.work
				}
				b.ReportMetric(0, "ns/op")
				reportMs(b, sum.mount, "mount-ms/op")
				reportMs(b, sum.work, "work-ms/op")
			})
		}
	}
}
