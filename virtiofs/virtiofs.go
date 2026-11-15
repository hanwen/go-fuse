package virtiofs

import (
	"errors"
	"io"
	"log"
	"net"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/vhostuser"
)

// ServeFS connects a FUSE filesystem to a virtio-fs device over a vhost-user
// socket.
//
// FUSE notifications (e.g. NotifyEntry, NotifyDelete) are not supported in the
// virtio-fs protocol: the virtqueue model only allows device-initiated messages
// in response to guest-provided buffers, and neither QEMU's virtiofsd nor the
// Linux kernel virtio-fs driver implement unsolicited device-to-driver
// notifications. Notification calls will return ENOSYS.
func ServeFS(sockpath string, rawFS fuse.RawFileSystem, opts *fuse.MountOptions) {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockpath, Net: "unix"})
	if err != nil {
		log.Fatal("Listen", err)
	}

	serve(l, rawFS, opts)
}

// serve handles connections on l until it is closed.
func serve(l *net.UnixListener, rawFS fuse.RawFileSystem, opts *fuse.MountOptions) {
	opts.DisableSplice = true
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			break
		}

		// Each connection is a new FUSE session, with its own
		// INIT, and DESTROY when the guest unmounts.
		//
		// TODO: the sessions share rawFS. A guest that
		// disconnects without RELEASE/FORGET (eg. it was killed)
		// leaks its inode references and open files.
		ps := fuse.NewProtocolServer(rawFS, opts, nil)

		dev := vhostuser.NewDevice(func(vqe *vhostuser.VirtqElem) int {
			n, _ := ps.HandleRequest(vqe.Read, vqe.Write)
			return n
		})
		srv := vhostuser.NewServer(conn, dev)
		srv.Debug = opts.Debug
		if err := srv.Serve(); err != nil && !errors.Is(err, io.EOF) {
			log.Printf("Serve: %v %T", err, err)
		}

		srv.Close()
	}
}
