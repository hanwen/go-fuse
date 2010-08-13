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
	managerReq := make(chan *managerRequest, 100)
	startManager(managerReq)
	defer close(managerReq)
	for {
		n, err := f.Read(buf)
		if err == os.EOF {
			break
		}
		if err != nil {
			errors <- os.NewError(fmt.Sprintf("Failed to read from fuse conn: %v", err))
			break
		}

		dispatch(fs, buf[0:n], managerReq, toW, errors)
	}
}

func dispatch(fs FileSystem, in_data []byte, mr chan *managerRequest, toW chan [][]byte, errors chan os.Error) {
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
		out, err = initFuse(fs, h, r, mr)
	case FUSE_FORGET:
		return
	case FUSE_GETATTR:
		out, err = getAttr(fs, h, r, mr)
	case FUSE_GETXATTR:
		out, err = getXAttr(h, r, mr)
	case FUSE_OPENDIR:
		out, err = openDir(h, r, mr)
	case FUSE_READDIR:
		out, err = readDir(h, r, mr)
	case FUSE_LOOKUP:
		out, err = lookup(h, r, mr)
	case FUSE_RELEASEDIR:
		out, err = releaseDir(h, r, mr)
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

func initFuse(fs FileSystem, h *InHeader, r io.Reader, mr chan *managerRequest) (data [][]byte, err os.Error) {
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

func getAttr(fs FileSystem, h *InHeader, r io.Reader, mr chan *managerRequest) (data [][]byte, err os.Error) {
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

func getXAttr(h *InHeader, r io.Reader, mr chan *managerRequest) (data [][]byte, err os.Error) {
	out := new(GetXAttrOut)
	data, err = serialize(h, OK, out)
	return
}

func openDir(h *InHeader, r io.Reader, mr chan *managerRequest) (data [][]byte, err os.Error) {
	in := new(OpenIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		data, _ = serialize(h, EIO, nil)
		return
	}
	fmt.Printf("FUSE_OPENDIR: %v\n", in)
	resp := makeManagerRequest(mr, h.NodeId, 0, openDirOp)
	err = resp.err
	if err != nil {
		data, err = serialize(h, EIO, nil)
		return
	}
	out := new(OpenOut)
	out.Fh = resp.fh
	res := OK
	data, err = serialize(h, res, out)
	return
}

func readDir(h *InHeader, r io.Reader, mr chan *managerRequest) (data [][]byte, err os.Error) {
	in := new(ReadIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		data, _ = serialize(h, EIO, nil)
		return
	}
	fmt.Printf("FUSE_READDIR: %v\n", in)
	resp := makeManagerRequest(mr, h.NodeId, in.Fh, getHandleOp)
	err = resp.err
	if err != nil {
		data, _ = serialize(h, EIO, nil)
		return
	}
	dirRespChan := make(chan *dirResponse, 1)
	fmt.Printf("Sending dir request\n")
	resp.dirReq <- &dirRequest{false, dirRespChan}
	fmt.Printf("receiving dir response\n")
	dirResp := <-dirRespChan
	fmt.Printf("received %v\n", dirResp)
	err = dirResp.err
	if err != nil {
		fmt.Printf("Err!\n")
		data, _ = serialize(h, EIO, nil)
		return
	}
	if dirResp.entries == nil {
		fmt.Printf("No entries\n")
		data, err = serialize(h, OK, nil)
		return
	}

	fmt.Printf("len(dirResp.entries): %v\n", len(dirResp.entries))
	buf := new(bytes.Buffer)
	off := uint64(0)
	for _, entry := range dirResp.entries {
		off++
		dirent := new(Dirent)
		dirent.Off = off
		dirent.Ino = h.NodeId + off
		dirent.NameLen = uint32(len(entry.name))
		dirent.Typ = (entry.mode & 0170000) >> 12
		err = binary.Write(buf, binary.LittleEndian, dirent)
		if err != nil {
			fmt.Printf("AAA!!! binary.Write failed\n")
			os.Exit(1)
		}
		buf.Write([]byte(entry.name))
		buf.WriteByte(0) // padding?
		n := (len(entry.name) + 1) % 8
		if n != 0 {
			buf.Write(make([]byte, 8-n))
		}
	}
	out := buf.Bytes()
	fmt.Printf("out: %v\n", out)
	res := OK
	data, err = serialize(h, res, out)
	return
}

func lookup(h *InHeader, r *bytes.Buffer, mr chan *managerRequest) (data [][]byte, err os.Error) {
	filename := string(r.Bytes())
	fmt.Printf("filename: %s\n", filename)
	out := new(EntryOut)
	out.NodeId = h.NodeId + 1
	out.Mode = S_IFDIR
	res := OK
	data, err = serialize(h, res, out)
	return
}

func releaseDir(h *InHeader, r io.Reader, mr chan *managerRequest) (data [][]byte, err os.Error) {
	in := new(ReleaseIn)
	err = binary.Read(r, binary.LittleEndian, in)
	if err != nil {
		data, err = serialize(h, EIO, nil)
		return
	}
	fmt.Printf("FUSE_RELEASEDIR: %v\n", in)
	resp := makeManagerRequest(mr, h.NodeId, in.Fh, closeDirOp)
	err = resp.err
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

type FileOp int

const (
	openDirOp   = FileOp(1)
	getHandleOp = FileOp(2)
	closeDirOp  = FileOp(3)
)

type managerRequest struct {
	nodeId uint64
	fh     uint64
	op     FileOp
	resp   chan *managerResponse
}

type managerResponse struct {
	fh     uint64
	dirReq chan *dirRequest
	err    os.Error
}

type dirEntry struct {
	name string
	mode uint32
}

type dirRequest struct {
	isClose bool
	resp    chan *dirResponse
}

type dirResponse struct {
	entries []*dirEntry
	err     os.Error
}

type dirHandle struct {
	fh     uint64
	nodeId uint64
	req    chan *dirRequest
}

type manager struct {
	dirHandles map[uint64]*dirHandle
	cnt        uint64
}

func startManager(requests chan *managerRequest) {
	m := new(manager)
	m.dirHandles = make(map[uint64]*dirHandle)
	go m.run(requests)
}

func makeManagerRequest(mr chan *managerRequest, nodeId uint64, fh uint64, op FileOp) (resp *managerResponse) {
	fmt.Printf("makeManagerRequest, nodeId = %d, fh = %d, op = %d\n", nodeId, fh, op)
	req := &managerRequest{nodeId, fh, op, make(chan *managerResponse, 1)}
	mr <- req
	resp = <-req.resp
	fmt.Printf("makeManagerRequest, resp: %v\n", resp)
	return
}

func (m *manager) run(requests chan *managerRequest) {
	var resp *managerResponse
	for req := range requests {
		switch req.op {
		case openDirOp:
			resp = m.openDir(req)
		case getHandleOp:
			resp = m.getHandle(req)
		case closeDirOp:
			resp = m.closeDir(req)
		default:
			resp := new(managerResponse)
			resp.err = os.NewError(fmt.Sprintf("Unknown FileOp: %v", req.op))
		}
		req.resp <- resp
	}
}

func (m *manager) openDir(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	m.cnt++
	h := new(dirHandle)
	h.fh = m.cnt
	h.nodeId = req.nodeId
	h.req = make(chan *dirRequest, 1)
	m.dirHandles[h.fh] = h
	go readDirRoutine(h.req)
	resp.fh = h.fh
	return
}

func (m *manager) getHandle(req *managerRequest) (resp *managerResponse) {
	fmt.Printf("getHandle, fh: %v\n", req.fh)
	resp = new(managerResponse)
	h, ok := m.dirHandles[req.fh]
	if !ok {
		resp.err = os.NewError(fmt.Sprintf("Unknown handle %d", req.fh))
		return
	}
	fmt.Printf("Handle found\n")
	resp.dirReq = h.req
	return
}

func (m *manager) closeDir(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	h, ok := m.dirHandles[req.fh]
	if !ok {
		resp.err = os.NewError(fmt.Sprintf("closeDir: unknown handle %d", req.fh))
		return
	}
	m.dirHandles[h.fh] = nil, false
	close(h.req)
	return
}

func readDirRoutine(requests chan *dirRequest) {
	defer close(requests)
	entries := []*dirEntry{
		&dirEntry{"lala111", S_IFDIR},
		&dirEntry{"bb", S_IFDIR},
		&dirEntry{"ddddd", S_IFDIR},
	}
	i := 0
	for req := range requests {
		if req.isClose {
			return
		}
		if i < len(entries) {
			req.resp <- &dirResponse{[]*dirEntry{entries[i]}, nil}
			i++
		} else {
			req.resp <- &dirResponse{nil, nil}
		}
	}
}
