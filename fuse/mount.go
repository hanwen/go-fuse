package fuse

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

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
