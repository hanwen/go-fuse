// A Go mirror of libfuse's hello.c

package main

import (
	"flag"
	"log"
	"github.com/hanwen/go-fuse/fuse"
	"os"
)

type HelloFs struct {
	fuse.DefaultFileSystem
}

func (me *HelloFs) GetAttr(name string, context *fuse.Context) (*os.FileInfo, fuse.Status) {
	switch name {
	case "file.txt":
		return &os.FileInfo{
			Mode: fuse.S_IFREG | 0644, Size: int64(len(name)),
		}, fuse.OK
	case "":
		return &os.FileInfo{
			Mode: fuse.S_IFDIR | 0755,
		}, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (me *HelloFs) OpenDir(name string, context *fuse.Context) (c chan fuse.DirEntry, code fuse.Status) {
	if name == "" {
		c = make(chan fuse.DirEntry, 1)
		c <- fuse.DirEntry{Name: "file.txt", Mode: fuse.S_IFREG}
		close(c)
		return c, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (me *HelloFs) Open(name string, flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	if name != "file.txt" {
		return nil, fuse.ENOENT
	}
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}
	return fuse.NewDataFile([]byte(name)), fuse.OK
}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	state, _, err := fuse.MountPathFileSystem(flag.Arg(0), &HelloFs{}, nil)
	if err != nil {
		log.Fatal("Mount fail: %v\n", err)
	}
	state.Loop()
}
