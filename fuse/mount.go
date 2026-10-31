package fuse

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Mount is a FUSE file system mounted at a path. It is separate from
// the Dev and the Server, so a process that does not serve the file
// system can still unmount it.
//
// A Mount is either created with NewMount and then mounted with
// Mount, or describes a file system that is already mounted, eg. by
// another process; NewMount gives such a Mount too. It is assumed to
// be mounted until Unmount succeeds.
//
// A Server given a Mount uses it in Server.Unmount and
// Server.WaitMount.
type Mount struct {
	path string

	// unmounted is set when Unmount succeeded, so a second
	// Unmount does not unmount a file system mounted at path
	// since. Mount clears it.
	unmounted bool
}

// NewMount returns the Mount at path, which is made absolute. It does
// not mount anything: call Mount for that, or use the result for a
// file system that is already mounted at path.
func NewMount(path string) (*Mount, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &Mount{path: path}, nil
}

// Mount mounts a FUSE file system at m's path, and returns its
// device. The kernel then waits for the INIT negotiation, which
// NewServerFromDev does. See NewServer for the ways of mounting. A
// Mount can be mounted again after Unmount.
//
// Set opts.Name for the subtype of the mount; unlike NewServer, Mount
// cannot derive it from the file system.
func (m *Mount) Mount(opts *MountOptions) (*Dev, error) {
	o := defaultMountOptions(opts)
	o.setDefaults(nil)
	ready := make(chan error, 1)
	fd, err := mount(m.path, &o, ready)
	if err != nil {
		return nil, err
	}
	m.unmounted = false
	return &Dev{fd: fd, ready: ready}, nil
}

// Path returns the absolute path of the mount.
func (m *Mount) Path() string {
	return m.path
}

// magic returns whether m is the /dev/fd/N syntax, rather than a
// directory.
func (m *Mount) magic() bool {
	return parseFuseFd(m.path) >= 0
}

// Unmount unmounts the file system. The kernel then aborts the
// connection, and servers for it exit. After it succeeds, further
// calls return nil until m is mounted again.
//
// It tries umount(2) first. On Linux, if that is not permitted, it
// runs fusermount -u.
//
// It does not work for the magic /dev/fd/N mount point syntax, as the
// real mount point is unknown. Use
//
//	fusermount -u /path/to/real/mountpoint
//
// in this case.
func (m *Mount) Unmount() (err error) {
	if m.unmounted {
		return nil
	}
	if m.magic() {
		return fmt.Errorf("Cannot unmount magic mountpoint %q. Please use `fusermount -u REALMOUNTPOINT` instead.", m.path)
	}
	delay := time.Duration(0)
	for try := 0; try < 5; try++ {
		err = unmount(m.path)
		if err == nil {
			break
		}

		// Sleep for a bit. This is not pretty, but there is
		// no way we can be certain that the kernel thinks all
		// open files have already been closed.
		delay = 2*delay + 5*time.Millisecond
		time.Sleep(delay)
	}
	if err != nil {
		return err
	}
	m.unmounted = true
	return nil
}

var reservedFDs []*os.File

func init() {
	// Both Darwin and Linux invoke a subprocess with one
	// inherited file descriptor to create the mount. To protect
	// against deadlock, we must ensure that file descriptor 3
	// never points to a FUSE filesystem. We do this by simply
	// grabbing fd 3 and never releasing it. (This is not
	// completely foolproof: a preceding init routine could grab fd 3,
	// and then release it later.)
	for {
		r, w, err := os.Pipe()
		if err != nil {
			panic(fmt.Sprintf("os.Pipe(): %v", err))
		}
		w.Close()
		if r.Fd() > 3 {
			r.Close()
			break
		}
		reservedFDs = append(reservedFDs, r)
	}
}

func getConnection(local *os.File) (int, error) {
	conn, err := net.FileConn(local)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	unixConn := conn.(*net.UnixConn)

	var data [4]byte
	control := make([]byte, 4*256)

	_, oobn, _, _, err := unixConn.ReadMsgUnix(data[:], control[:])
	if err != nil {
		return 0, err
	}

	messages, err := syscall.ParseSocketControlMessage(control[:oobn])
	if err != nil {
		return 0, err
	}
	if len(messages) != 1 {
		return 0, fmt.Errorf("getConnection: expect 1 control message, got %#v", messages)
	}
	message := messages[0]

	fds, err := syscall.ParseUnixRights(&message)
	if err != nil {
		return 0, err
	}
	if len(fds) != 1 {
		return 0, fmt.Errorf("getConnection: expect 1 fd, got %#v", fds)
	}
	fd := fds[0]

	if fd < 0 {
		return 0, fmt.Errorf("getConnection: fd < 0: %d", fd)
	}
	return fd, nil
}
