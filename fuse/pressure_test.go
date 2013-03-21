package fuse

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"
)

var _ = log.Println

// This test checks that highly concurrent loads don't use a lot of
// memory if it is not needed: The input buffer needs to accomodata
// the max write size, but it is only really needed when we are
// processing writes.
type DelayFs struct {
	DefaultFileSystem

	fileRegex *regexp.Regexp
	dirRegex  *regexp.Regexp
}

func (d *DelayFs) GetAttr(name string, c *Context) (*Attr, Status) {
	if name == "" || d.dirRegex.MatchString(name) {
		return &Attr{Mode: S_IFDIR | 0755}, OK
	}
	if d.fileRegex.MatchString(name) {
		time.Sleep(time.Second)
		return &Attr{Mode: S_IFREG | 0644}, OK
	}
	return nil, ENOENT
}

func TestMemoryPressure(t *testing.T) {
	fs := &DelayFs{
		fileRegex: regexp.MustCompile("^dir[0-9]*/file[0-9]*$"),
		dirRegex:  regexp.MustCompile("^dir[0-9]*$"),
	}

	dir, err := ioutil.TempDir("", "go-fuse-pressure_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(dir)
	nfs := NewPathNodeFs(fs, nil)
	o := &FileSystemOptions{PortableInodes: true}

	conn := NewFileSystemConnector(nfs, o)
	state := NewMountState(conn)
	bufs := NewBufferPool()

	err = state.Mount(dir, &MountOptions{Buffers: bufs})
	if err != nil {
		t.Fatalf("mount failed: %v", err)
	}
	state.Debug = VerboseTest()

	go state.Loop()
	defer state.Unmount()

	// Wait for FS to get ready.
	os.Lstat(dir)

	var wg sync.WaitGroup
	for i := 0; i < 10*_MAX_READERS; i++ {
		wg.Add(1)
		go func(x int) {
			fn := fmt.Sprintf("%s/dir%d/file%d", dir, x, x)
			_, err := os.Lstat(fn)
			if err != nil {
				t.Errorf("parallel stat %q: %v", fn, err)
			}
			wg.Done()
		}(i)
	}
	time.Sleep(100 * time.Millisecond)

	state.reqMu.Lock()
	bufs.lock.Lock()
	created := bufs.createdBuffers + state.outstandingReadBufs
	bufs.lock.Unlock()
	state.reqMu.Unlock()

	t.Logf("Have %d read bufs", state.outstandingReadBufs)
	// +1 due to batch forget?
	if created > _MAX_READERS+1 {
		t.Errorf("created %d buffers, max reader %d", created, _MAX_READERS)
	}

	wg.Wait()
}
