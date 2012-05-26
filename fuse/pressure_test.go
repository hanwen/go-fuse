package fuse

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// This test checks that highly concurrent loads don't use a lot of
// memory if it is not needed: The input buffer needs to accomodata
// the max write size, but it is only really needed when we are
// processing writes.
type DelayFs struct {
	DefaultFileSystem
}

func (d *DelayFs) GetAttr(name string, c *Context) (*Attr, Status) {
	if name == "" || strings.HasSuffix(name, "dir") {
		return &Attr{Mode: S_IFDIR | 0755}, OK
	}
	time.Sleep(time.Second)
	return &Attr{Mode: S_IFREG | 0644}, OK
}

func TestMemoryPressure(t *testing.T) {
	fs := &DelayFs{}

	dir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	nfs := NewPathNodeFs(fs, nil)
	o := &FileSystemOptions{PortableInodes: true}

	conn := NewFileSystemConnector(nfs, o)
	state := NewMountState(conn)
	bufs := NewBufferPool()
	
	err = state.Mount(dir, &MountOptions{Buffers: bufs})
	if err != nil {
		t.Fatalf("mount failed: %v", err)
	}
	go state.Loop()
	defer state.Unmount()

	state.Debug = VerboseTest()

	// Wait for FS to get ready.
	os.Lstat(dir)

	var wg sync.WaitGroup
	for i := 0; i < 10*_MAX_READERS; i++ {
		wg.Add(1)
		go func(x int) {
			fn := fmt.Sprintf("%s/%ddir/file%d", dir, x, x)
			_, err := os.Lstat(fn)
			if err != nil {
				t.Errorf("parallel stat %q: %v", fn, err)
			}
			wg.Done()
		}(i)
	}
	time.Sleep(100 * time.Millisecond)
	created := bufs.createdBuffers + state.outstandingReadBufs

	t.Logf("Have %d read bufs", state.outstandingReadBufs)
	if created > 2*_MAX_READERS {
		t.Errorf("created %d buffers, max reader %d", created, _MAX_READERS)
	}

	wg.Wait()
}
