package fuse

// Written with a look to http://ptspts.blogspot.com/2009/11/fuse-protocol-tutorial-for-linux-26.html

import (
	"fmt"
	"net"
	"os"
	"path"
)

type FileSystem interface {
}

type MountPoint struct {
	mountPoint string
	f *os.File
}

// Mount create a fuse fs on the specified mount point.
func Mount(mountPoint string, fs FileSystem) (m *MountPoint, err os.Error) {
	local, remote, err := net.Socketpair("unixgram")
	if err != nil {
		return
	}
	defer local.Close()
	defer remote.Close()

	mountPoint = path.Clean(mountPoint)
	if !path.Rooted(mountPoint) {
		cwd, err := os.Getwd()
		if err != nil {
			return
		}
		mountPoint = path.Clean(path.Join(cwd, mountPoint))
	}
	pid, err := os.ForkExec("/bin/fusermount",
			[]string { "/bin/fusermount", mountPoint },
			[]string { "_FUSE_COMMFD=3" },
			"",
			[]*os.File { nil, nil, os.Stderr, remote.File() })
	if err != nil {
		return
	}
	w, err := os.Wait(pid, 0)
	if err != nil {
		return
	}
	if w.ExitStatus() != 0 {
		return nil, os.NewError(fmt.Sprintf("fusermount exited with code %d\n", w.ExitStatus()))
	}
	f, err := getFuseConn(local)
	if err != nil {
		return
	}
	m = &MountPoint { mountPoint, f }
	return
}

func (m *MountPoint) Unmount() (err os.Error) {
	pid, err := os.ForkExec("/bin/fusermount",
			[]string { "/bin/fusermount", "-u", "m", m.mountPoint },
			nil,
			"",
			[]*os.File { nil, nil, os.Stderr })
	if err != nil {
		return
	}
	w, err := os.Wait(pid, 0)
	if err != nil {
		return
	}
	if w.ExitStatus() != 0 {
		return os.NewError(fmt.Sprintf("fusermount exited with code %d\n", w.ExitStatus()))
	}
	m.f.Close()
	return
}

func getFuseConn(local net.Conn) (f * os.File, err os.Error) {
	var fd int
	return nil, os.NewError(fmt.Sprintf("receive_fuse_conn failed with errno: %d", errno))
//	f = os.NewFile(fd, "fuse-conn")
//	return
}


