// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v3/internal/testutil"
	"golang.org/x/sys/unix"
)

// mountSwapped looks up prime through the mount, then replaces swap in
// the backing tree with symlinks into outside, behind the kernel's
// cached entries.
func mountSwapped(t *testing.T, setup func(orig, outside string), prime, swap []string) (mnt, outside string) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig")
	mnt = filepath.Join(dir, "mnt")
	outside = filepath.Join(dir, "outside")
	for _, d := range []string{orig, mnt, outside} {
		if err := os.Mkdir(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	setup(orig, outside)

	root, err := NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	hour := time.Hour
	var zero time.Duration
	opts := &Options{EntryTimeout: &hour, NegativeTimeout: &hour, AttrTimeout: &zero}
	opts.Debug = testutil.VerboseTest()
	srv, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })

	for _, p := range prime {
		os.Lstat(filepath.Join(mnt, p))
	}
	for _, p := range swap {
		o := filepath.Join(orig, p)
		if err := os.Rename(o, o+".old"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, p), o); err != nil {
			t.Fatal(err)
		}
	}
	return mnt, outside
}

func swapWriteFile(t *testing.T, p, content string) {
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func swapMkdir(t *testing.T, p string) {
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
}

func wantMissing(t *testing.T, p string) {
	if _, err := os.Lstat(p); err == nil {
		t.Errorf("%s was created outside the loopback root", p)
	}
}

func wantExists(t *testing.T, p string) {
	if _, err := os.Lstat(p); err != nil {
		t.Errorf("%s outside the loopback root: %v", p, err)
	}
}

func wantContent(t *testing.T, p, want string) {
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s: got %q, want %q", p, got, want)
	}
}

func wantMode(t *testing.T, p string, want os.FileMode) {
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s: mode %o, want %o", p, got, want)
	}
}

func wantNoEntry(t *testing.T, dir, name string) {
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Logf("ReadDir: %v", err)
		return
	}
	if slices.ContainsFunc(es, func(e os.DirEntry) bool { return e.Name() == name }) {
		t.Errorf("%s lists %q from outside the loopback root", dir, name)
	}
}

func TestLoopbackSymlinkSwapFinal(t *testing.T) {
	setup := func(orig, outside string) {
		swapWriteFile(t, filepath.Join(orig, "f"), "orig")
		swapMkdir(t, filepath.Join(orig, "d"))
		swapWriteFile(t, filepath.Join(outside, "f"), "secret")
		swapMkdir(t, filepath.Join(outside, "d"))
		swapWriteFile(t, filepath.Join(outside, "d", "secret"), "secret")
	}
	names := []string{"f", "d", "new"}

	for name, fn := range map[string]func(t *testing.T, mnt, outside string){
		"chmod": func(t *testing.T, mnt, outside string) {
			os.Chmod(filepath.Join(mnt, "f"), 0600)
			wantMode(t, filepath.Join(outside, "f"), 0644)
		},
		"truncate": func(t *testing.T, mnt, outside string) {
			os.Truncate(filepath.Join(mnt, "f"), 0)
			wantContent(t, filepath.Join(outside, "f"), "secret")
		},
		"create": func(t *testing.T, mnt, outside string) {
			if f, err := os.OpenFile(filepath.Join(mnt, "new"), os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.Close()
			}
			wantMissing(t, filepath.Join(outside, "new"))
		},
		"readdir": func(t *testing.T, mnt, outside string) {
			wantNoEntry(t, filepath.Join(mnt, "d"), "secret")
		},
	} {
		t.Run(name, func(t *testing.T) {
			mnt, outside := mountSwapped(t, setup, names, names)
			fn(t, mnt, outside)
		})
	}
}

func TestLoopbackSymlinkSwapParent(t *testing.T) {
	setup := func(orig, outside string) {
		for _, d := range []string{orig, outside} {
			swapMkdir(t, filepath.Join(d, "d"))
			swapMkdir(t, filepath.Join(d, "d", "sub"))
		}
		swapWriteFile(t, filepath.Join(orig, "d", "f"), "orig")
		swapWriteFile(t, filepath.Join(outside, "d", "f"), "secret")
		swapWriteFile(t, filepath.Join(outside, "d", "secret"), "secret")
	}
	prime := []string{"d", "d/f", "d/sub"}
	swap := []string{"d"}

	for name, fn := range map[string]func(t *testing.T, mnt, outside string){
		"create": func(t *testing.T, mnt, outside string) {
			if f, err := os.Create(filepath.Join(mnt, "d", "new")); err == nil {
				f.Close()
			}
			wantMissing(t, filepath.Join(outside, "d", "new"))
		},
		"mkdir": func(t *testing.T, mnt, outside string) {
			os.Mkdir(filepath.Join(mnt, "d", "new"), 0755)
			wantMissing(t, filepath.Join(outside, "d", "new"))
		},
		"mknod": func(t *testing.T, mnt, outside string) {
			syscall.Mkfifo(filepath.Join(mnt, "d", "new"), 0644)
			wantMissing(t, filepath.Join(outside, "d", "new"))
		},
		"symlink": func(t *testing.T, mnt, outside string) {
			os.Symlink("x", filepath.Join(mnt, "d", "new"))
			wantMissing(t, filepath.Join(outside, "d", "new"))
		},
		"link": func(t *testing.T, mnt, outside string) {
			os.Link(filepath.Join(mnt, "d", "f"), filepath.Join(mnt, "d", "new"))
			wantMissing(t, filepath.Join(outside, "d", "new"))
		},
		"unlink": func(t *testing.T, mnt, outside string) {
			os.Remove(filepath.Join(mnt, "d", "f"))
			wantExists(t, filepath.Join(outside, "d", "f"))
		},
		"rmdir": func(t *testing.T, mnt, outside string) {
			os.Remove(filepath.Join(mnt, "d", "sub"))
			wantExists(t, filepath.Join(outside, "d", "sub"))
		},
		"rename": func(t *testing.T, mnt, outside string) {
			os.Rename(filepath.Join(mnt, "d", "f"), filepath.Join(mnt, "d", "new"))
			wantExists(t, filepath.Join(outside, "d", "f"))
		},
		"chmod": func(t *testing.T, mnt, outside string) {
			os.Chmod(filepath.Join(mnt, "d", "f"), 0600)
			wantMode(t, filepath.Join(outside, "d", "f"), 0644)
		},
		"truncate": func(t *testing.T, mnt, outside string) {
			os.Truncate(filepath.Join(mnt, "d", "f"), 0)
			wantContent(t, filepath.Join(outside, "d", "f"), "secret")
		},
		"utimes": func(t *testing.T, mnt, outside string) {
			epoch := time.Unix(0, 0)
			os.Chtimes(filepath.Join(mnt, "d", "f"), epoch, epoch)
			fi, err := os.Lstat(filepath.Join(outside, "d", "f"))
			if err != nil {
				t.Fatal(err)
			}
			if fi.ModTime().Equal(epoch) {
				t.Errorf("mtime of %s changed", filepath.Join(outside, "d", "f"))
			}
		},
		"setxattr": func(t *testing.T, mnt, outside string) {
			unix.Setxattr(filepath.Join(mnt, "d", "f"), "user.swap", []byte("x"), 0)
			if _, err := unix.Getxattr(filepath.Join(outside, "d", "f"), "user.swap", nil); err == nil {
				t.Errorf("xattr set outside the loopback root")
			}
		},
		"stat": func(t *testing.T, mnt, outside string) {
			fi, err := os.Stat(filepath.Join(mnt, "d", "f"))
			if err == nil && fi.Size() == int64(len("secret")) {
				t.Errorf("stat returned the size of the file outside the loopback root")
			}
		},
		"readdir": func(t *testing.T, mnt, outside string) {
			wantNoEntry(t, filepath.Join(mnt, "d"), "secret")
		},
	} {
		t.Run(name, func(t *testing.T) {
			mnt, outside := mountSwapped(t, setup, prime, swap)
			fn(t, mnt, outside)
		})
	}
}
