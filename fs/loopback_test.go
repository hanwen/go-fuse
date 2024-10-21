package fs

import (
	"context"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/renameat"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/sys/unix"
)

func TestRenameExchange(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	tc.writeOrig("file", "hello", 0644)
	tc.writeOrig("dir/file", "x", 0644)

	f1, err := syscall.Open(tc.mntDir+"/", syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	defer syscall.Close(f1)
	f2, err := syscall.Open(tc.mntDir+"/dir", syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer syscall.Close(f2)

	var before1, before2 unix.Stat_t
	if err := unix.Fstatat(f1, "file", &before1, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	if err := unix.Fstatat(f2, "file", &before2, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}

	if err := renameat.Renameat(f1, "file", f2, "file", renameat.RENAME_EXCHANGE); err != nil {
		if err == unix.ENOSYS {
			t.Skip("rename EXCHANGE not support on current system")
		} else {
			t.Errorf("rename EXCHANGE: %v", err)
		}
	}

	var after1, after2 unix.Stat_t
	if err := unix.Fstatat(f1, "file", &after1, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	if err := unix.Fstatat(f2, "file", &after2, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	clearCtime := func(s *unix.Stat_t) {
		s.Ctim.Sec = 0
		s.Ctim.Nsec = 0
	}

	clearCtime(&after1)
	clearCtime(&after2)
	clearCtime(&before2)
	clearCtime(&before1)
	if diff := pretty.Compare(after1, before2); diff != "" {
		t.Errorf("after1, before2: %s", diff)
	}
	if !reflect.DeepEqual(after2, before1) {
		t.Errorf("after2, before1: %#v, %#v", after2, before1)
	}

	root := tc.loopback.EmbeddedInode().Root()
	ino1 := root.GetChild("file")
	if ino1 == nil {
		t.Fatalf("root.GetChild(%q): null inode", "file")
	}
	ino2 := root.GetChild("dir").GetChild("file")
	if ino2 == nil {
		t.Fatalf("dir.GetChild(%q): null inode", "file")
	}
	if ino1.StableAttr().Ino != after1.Ino {
		t.Errorf("got inode %d for %q, want %d", ino1.StableAttr().Ino, "file", after1.Ino)
	}
	if ino2.StableAttr().Ino != after2.Ino {
		t.Errorf("got inode %d for %q want %d", ino2.StableAttr().Ino, "dir/file", after2.Ino)
	}
}

func TestLoopbackNonRoot(t *testing.T) {
	backing := t.TempDir()
	content := []byte("hello")
	if err := os.WriteFile(backing+"/file.txt", content, 0666); err != nil {
		t.Fatal(err)
	}

	root := &Inode{}
	mnt, _ := testMount(t, root, &Options{
		OnAdd: func(ctx context.Context) {
			lnode, err := NewLoopbackRoot(backing)
			if err != nil {
				return
			}
			sub := root.NewPersistentInode(ctx, lnode, StableAttr{Mode: fuse.S_IFDIR})
			root.AddChild("sub", sub, true)
		},
	})

	fi, err := os.Lstat(mnt + "/sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != int64(len(content)) {
		t.Errorf("got %d bytes, want %d", fi.Size(), len(content))
	}
}
