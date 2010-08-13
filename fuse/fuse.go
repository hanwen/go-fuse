package fuse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	bufSize = 66000
)

type FileSystem interface {
	Init(in *InitIn) (out *InitOut, code Error, err os.Error)
	GetAttr(h *InHeader, in *GetAttrIn) (out *AttrOut, code Error, err os.Error)
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

		dispatch(fs, buf[0:n], toW, errors)
	}
}

func dispatch(fs FileSystem, in_data []byte, toW chan [][]byte, errors chan os.Error) {
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
	var out [][]byte
	fmt.Printf("Opcode: %v, NodeId: %v, h: %v\n", h.Opcode, h.NodeId, h)
	switch h.Opcode {
	case FUSE_INIT:
		out, err = initFuse(fs, h, r)
	case FUSE_FORGET:
		return
	case FUSE_GETATTR:
		out, err = getAttr(fs, h, r)
	case FUSE_GETXATTR:
		out, err = getXAttr(h, r)
	case FUSE_OPENDIR:
		out, err = openDir(h, r)
	case FUSE_READDIR:
		out, err = readDir(h, r)
	case FUSE_LOOKUP:
		out, err = lookup(h, r)
	case FUSE_RELEASEDIR:
		out, err = releaseDir(h, r)
	default:
		errors <- os.NewError(fmt.Sprintf("Unsupported OpCode: %d", h.Opcode))
		out, err = serialize(h, EIO, nil)
	}
	if err != nil {
		errors <- err
		out, err = serialize(h, EIO, nil)
	}
	if out == nil || len(out) == 0 {
		fmt.Printf("out is empty\n")
		return
	}

	fmt.Printf("Sending to writer: %v\n", out)
	toW <- out
}

func serialize(h *InHeader, res Error, out interface{}) (data [][]byte, err os.Error) {
	b := new(bytes.Buffer)
	out_data := make([]byte, 0)
	fmt.Printf("OpCode: %v result: %v\n", h.Opcode, res)
	if out != nil && res == OK {
		fmt.Printf("out = %v, out == nil: %v\n", out, out == nil)
		err = binary.Write(b, binary.LittleEndian, out)
		if err == nil {
			out_data = b.Bytes()
		} else {
			err = os.NewError(fmt.Sprintf("Can serialize out: %v", err))
			return
		}
	}
	fmt.Printf("out_data: %v, len(out_data): %d, SizeOfOutHeader: %d\n", out_data, len(out_data), SizeOfOutHeader)
	var hout OutHeader
	hout.Unique = h.Unique
	hout.Error = int32(res)
	hout.Length = uint32(len(out_data) + SizeOfOutHeader)
	b = new(bytes.Buffer)
	err = binary.Write(b, binary.LittleEndian, &hout)
	if err != nil {
		return
	}
	_, _ = b.Write(out_data)
	data = [][]byte{b.Bytes()}
	return
}

func initFuse(fs FileSystem, h *InHeader, r io.Reader) (data [][]byte, err os.Error) {
	in := new(InitIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		return
	}
	fmt.Printf("in: %v\n", in)
	var out *InitOut
	out, res, err := fs.Init(in)
	if err != nil {
		return
	}
	data, err = serialize(h, res, out)
	return
}

func getAttr(fs FileSystem, h *InHeader, r io.Reader) (data [][]byte, err os.Error) {
	in := new(GetAttrIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		return
	}
	fmt.Printf("FUSE_GETATTR: %v\n", in)
	var out *AttrOut
	out, res, err := fs.GetAttr(h, in)
	if err != nil {
		return
	}
	data, err = serialize(h, res, out)
	return
}

func getXAttr(h *InHeader, r io.Reader) (data [][]byte, err os.Error) {
	out := new(GetXAttrOut)
	data, err = serialize(h, OK, out)
	return
}

func openDir(h *InHeader, r io.Reader) (data [][]byte, err os.Error) {
	in := new(OpenIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		return
	}
	fmt.Printf("FUSE_OPENDIR: %v\n", in)
	out := new(OpenOut)
	out.Fh = 1
	was = false
	res := OK
	data, err = serialize(h, res, out)
	return
}

func readDir(h *InHeader, r io.Reader) (data [][]byte, err os.Error) {
	if was {
		data, err = serialize(h, OK, nil)
		return
	}
	in := new(ReadIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		return
	}
	fmt.Printf("FUSE_READDIR: %v\n", in)

	dirent := new(Dirent)
	dirent.Off = 1
	dirent.Ino = h.NodeId
	dirent.NameLen = 7
	dirent.Typ = (S_IFDIR & 0170000) >> 12
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.LittleEndian, dirent)
	if err != nil {
		fmt.Printf("AAA!!! binary.Write failed\n")
		os.Exit(1)
	}
	buf.Write([]byte("hello12"))
	buf.WriteByte(0)
	out := buf.Bytes()
	was = true
	res := OK
	data, err = serialize(h, res, out)
	return
}

func lookup(h *InHeader, r *bytes.Buffer) (data [][]byte, err os.Error) {
	filename := string(r.Bytes())
	fmt.Printf("filename: %s\n", filename)
	out := new(EntryOut)
	out.NodeId = h.NodeId + 1
	out.Mode = S_IFDIR
	res := OK
	data, err = serialize(h, res, out)
	return
}

func releaseDir(h *InHeader, r io.Reader) (data [][]byte, err os.Error) {
	return
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
