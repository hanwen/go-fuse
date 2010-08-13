package fuse

// Written with a look to http://ptspts.blogspot.com/2009/11/fuse-protocol-tutorial-for-linux-26.html

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	bufSize = 66000
)

type FileSystem interface {
	Init(in *InitIn) (out *InitOut, code Error)
	GetAttr(h *InHeader, in *GetAttrIn) (out *AttrOut, code Error)
}

var was bool

type Mounted interface {
	Unmount() (err os.Error)
}

func Mount(mountPoint string, fs FileSystem) (m Mounted, err os.Error, errors chan os.Error) {
	f, m, err := mount(mountPoint)
	if err != nil {
		return
	}
	errors = make(chan os.Error, 100)
	go loop(f, fs, errors)
	return
}

func loop(f *os.File, fs FileSystem, errors chan os.Error) {
	buf := make([]byte, bufSize)
	defer close(errors)
	toW := make(chan [][]byte, 100)
	defer close(toW)
	go writer(f, toW, errors)
	for {
		n, err := f.Read(buf)
		if err == os.EOF {
			break
		}
		if err != nil {
			errors <- os.NewError(fmt.Sprintf("Failed to read from fuse conn: %v", err))
			break
		}

		handle(fs, buf[0:n], toW, errors)
	}
}

func handle(fs FileSystem, in_data []byte, toW chan [][]byte, errors chan os.Error) {
	fmt.Printf("in_data: %v\n", in_data)
	r := bytes.NewBuffer(in_data)
	h := new(InHeader)
	err := binary.Read(r, binary.LittleEndian, h)
	if err == os.EOF {
		err = os.NewError(fmt.Sprintf("MountPoint, handle: can't read a header, in_data: %v", in_data))
	}
	if err != nil {
		errors <- err
		return
	}
	var out interface{}
	var result Error = OK
	fmt.Printf("Opcode: %v, NodeId: %v, h: %v\n", h.Opcode, h.NodeId, h)
	switch h.Opcode {
	case FUSE_INIT:
		in := new(InitIn)
		err = binary.Read(r, binary.LittleEndian, in)
		if err != nil {
			break
		}
		fmt.Printf("in: %v\n", in)
		var init_out *InitOut
		init_out, result = fs.Init(in)
		if init_out != nil {
			out = init_out
		}
	case FUSE_FORGET:
		return

	case FUSE_GETATTR:
		in := new(GetAttrIn)
		err = binary.Read(r, binary.LittleEndian, in)
		if err != nil {
			break
		}
		fmt.Printf("FUSE_GETATTR: %v\n", in)
		var attr_out *AttrOut
		attr_out, result = fs.GetAttr(h, in)
		if attr_out != nil {
			out = attr_out
		}
	case FUSE_GETXATTR:
		result = OK
		out = new(GetXAttrOut)

	case FUSE_OPENDIR:
		in := new(OpenIn)
		err = binary.Read(r, binary.LittleEndian, in)
		if err != nil {
			break
		}
		fmt.Printf("FUSE_OPENDIR: %v\n", in)
		var open_out *OpenOut
		open_out = new(OpenOut)
		open_out.Fh = 1
		out = open_out
		was = false

	case FUSE_READDIR:
		if was {
			break
		}
		in := new(ReadIn)
		err = binary.Read(r, binary.LittleEndian, in)
		if err != nil {
			break
		}
		fmt.Printf("FUSE_READDIR: %v\n", in)

		dirent := new(Dirent)
		dirent.Off = 1
		dirent.Ino = h.NodeId
		dirent.NameLen = 7
		dirent.Typ = (S_IFDIR & 0170000) >> 12;
		buf := new(bytes.Buffer)
		err = binary.Write(buf, binary.LittleEndian, dirent)
		if err != nil {
			fmt.Printf("AAA!!! binary.Write failed\n")
			os.Exit(1)
		}
		buf.Write([]byte("hello12"))
		buf.WriteByte(0)
		out = buf.Bytes()
		was = true
	case FUSE_LOOKUP:
		filename := string(r.Bytes())
		fmt.Printf("filename: %s\n", filename)
		entry_out := new(EntryOut)
		entry_out.NodeId = h.NodeId + 1
		entry_out.Mode = S_IFDIR
		out = entry_out
	case FUSE_RELEASEDIR:
		return

	default:
		errors <- os.NewError(fmt.Sprintf("Unsupported OpCode: %d", h.Opcode))
		result = EIO
	}
	if err != nil {
		errors <- err
		out = nil
		result = EIO
		// Add sending result msg with error
	}
	b := new(bytes.Buffer)
	out_data := make([]byte, 0)
	fmt.Printf("OpCode: %v result: %v\n", h.Opcode, result)
	if out != nil && result == OK {
		fmt.Printf("out = %v, out == nil: %v\n", out, out == nil)
		err = binary.Write(b, binary.LittleEndian, out)
		if err == nil {
			out_data = b.Bytes()
		} else {
			errors <- os.NewError(fmt.Sprintf("Can serialize out: %v", err))
		}
	}
	fmt.Printf("out_data: %v, len(out_data): %d, SizeOfOutHeader: %d\n", out_data, len(out_data), SizeOfOutHeader)
	var hout OutHeader
	hout.Unique = h.Unique
	hout.Error = int32(result)
	hout.Length = uint32(len(out_data) + SizeOfOutHeader)
	b = new(bytes.Buffer)
	err = binary.Write(b, binary.LittleEndian, &hout)
	if err != nil {
		errors <- err
		return
	}
	_, _ = b.Write(out_data)
	fmt.Printf("Sending to writer: %v\n", b.Bytes())
	toW <- [][]byte{b.Bytes()}
}

func writer(f *os.File, in chan [][]byte, errors chan os.Error) {
	fd := f.Fd()
	for packet := range in {
		fmt.Printf("writer, packet: %v\n", packet)
		_, err := Writev(fd, packet)
		if err != nil {
			errors <- os.NewError(fmt.Sprintf("writer: Writev failed, err: %v", err))
			continue
		}
		fmt.Printf("writer: OK\n")
	}
}

