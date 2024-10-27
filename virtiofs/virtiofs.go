package virtiofs

import (
	"log"
	"net"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/vhostuser"
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

	opts.DisableSplice = true
	ps := fuse.NewProtocolServer(rawFS, opts)
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			break
		}

		dev := vhostuser.NewDevice(func(vqe *vhostuser.VirtqElem) int {
			n, _ := ps.HandleRequest(vqe.Read, vqe.Write)
			return n
		})
		//dev.Debug = true
		srv := vhostuser.NewServer(conn, dev)
		srv.Debug = true
		if err := srv.Serve(); err != nil {
			log.Printf("Serve: %v %T", err, err)
		}

		srv.Close()
	}
}
