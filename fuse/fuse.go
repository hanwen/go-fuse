package fuse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"strings"
)

const (
	bufSize = 66000
)

type File interface {
	ReadAt(p []byte, off int64) (n int, err os.Error)
	Close() (status Status)
}

type FileSystem interface {
	List(parent string) (names []string, status Status)
	GetAttr(path string) (out Attr, status Status)
	Open(path string) (file File, status Status)
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
	startManager(fs, managerReq)
	c := &managerClient{managerReq}
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

		go handle(fs, buf[0:n], c, toW, errors)
	}
}

func handle(fs FileSystem, in_data []byte, c *managerClient, toW chan [][]byte, errors chan os.Error) {
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
	out := dispatch(fs, h, r, c, errors)
	if out == nil {
		fmt.Printf("out is nil")
		return
	}
	fmt.Printf("Sending to writer: %v\n", out)
	toW <- out
}

func dispatch(fs FileSystem, h *InHeader, r *bytes.Buffer, c *managerClient, errors chan os.Error) (out [][]byte) {
	fmt.Printf("Opcode: %v, NodeId: %v, h: %v\n", h.Opcode, h.NodeId, h)
	switch h.Opcode {
	case FUSE_INIT:
		return parseInvoke(initFuse, fs, h, r, c, new(InitIn))
	case FUSE_FORGET:
		return nil
	case FUSE_GETATTR:
		return parseInvoke(getAttr, fs, h, r, c, new(GetAttrIn))
	case FUSE_GETXATTR:
		return parseInvoke(getXAttr, fs, h, r, c, new(GetXAttrIn))
	case FUSE_OPENDIR:
		return parseInvoke(openDir, fs, h, r, c, new(OpenIn))
	case FUSE_READDIR:
		return parseInvoke(readDir, fs, h, r, c, new(ReadIn))
	case FUSE_LOOKUP:
		out, status := lookup(h, r, c)
		return serialize(h, status, out)
	case FUSE_RELEASEDIR:
		return parseInvoke(releaseDir, fs, h, r, c, new(ReleaseIn))
	case FUSE_OPEN:
		return parseInvoke(open, fs, h, r, c, new(OpenIn))
	case FUSE_READ:
		return parseInvoke(read, fs, h, r, c, new(ReadIn))
	case FUSE_FLUSH:
		return parseInvoke(flush, fs, h, r, c, new(FlushIn))
	case FUSE_RELEASE:
		return parseInvoke(release, fs, h, r, c, new(ReleaseIn))
	default:
		errors <- os.NewError(fmt.Sprintf("Unsupported OpCode: %d", h.Opcode))
		return serialize(h, ENOSYS, nil)
	}
	return
}

func parse(b *bytes.Buffer, data interface{}) bool {
	err := binary.Read(b, binary.LittleEndian, data)
	if err == nil {
		return true
	}
	if err == os.EOF {
		return false
	}
	panic(fmt.Sprintf("Cannot parse %v", data))
}

type handler func(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (out interface{}, status Status)

func parseInvoke(f handler, fs FileSystem, h *InHeader, b *bytes.Buffer, c *managerClient, ing interface{}) [][]byte {
	if parse(b, ing) {
		out, status := f(fs, h, ing, c)
		if status != OK {
			out = nil
		}
		return serialize(h, status, out)
	}
	return serialize(h, EIO, nil)
}

func serialize(h *InHeader, res Status, out interface{}) (data [][]byte) {
	b := new(bytes.Buffer)
	out_data := make([]byte, 0)
	fmt.Printf("OpCode: %v result: %v\n", h.Opcode, res)
	if out != nil && res == OK {
		fmt.Printf("out = %v, out == nil: %v\n", out, out == nil)
		err := binary.Write(b, binary.LittleEndian, out)
		if err == nil {
			out_data = b.Bytes()
		} else {
			panic(fmt.Sprintf("Can't serialize out: %v, err: %v", out, err))
		}
	}
	fmt.Printf("out_data: %v, len(out_data): %d, SizeOfOutHeader: %d\n",
		out_data, len(out_data), SizeOfOutHeader)
	var hout OutHeader
	hout.Unique = h.Unique
	hout.Status = res
	hout.Length = uint32(len(out_data) + SizeOfOutHeader)
	b = new(bytes.Buffer)
	err := binary.Write(b, binary.LittleEndian, &hout)
	if err != nil {
		panic("Can't serialize OutHeader")
	}
	_, _ = b.Write(out_data)
	data = [][]byte{b.Bytes()}
	return
}

func initFuse(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in, _ := ing.(*InitIn)
	fmt.Printf("in: %v\n", in)
	if in.Major != FUSE_KERNEL_VERSION {
		fmt.Printf("Major versions does not match. Given %d, want %d\n", in.Major, FUSE_KERNEL_VERSION)
		return nil, EIO
	}
	if in.Minor < FUSE_KERNEL_MINOR_VERSION {
		fmt.Printf("Minor version is less than we support. Given %d, want at least %d\n", in.Minor, FUSE_KERNEL_MINOR_VERSION)
		return nil, EIO
	}
	out := new(InitOut)
	out.Major = FUSE_KERNEL_VERSION
	out.Minor = FUSE_KERNEL_MINOR_VERSION
	out.MaxReadAhead = in.MaxReadAhead
	out.Flags = FUSE_ASYNC_READ | FUSE_POSIX_LOCKS
	out.MaxWrite = 65536
	return out, OK
}

func getAttr(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*GetAttrIn)
	fmt.Printf("FUSE_GETATTR: %v, Fh: %d\n", in, in.Fh)
	out := new(AttrOut)
	resp := c.getPath(h.NodeId)
	if resp.status != OK {
		return nil, resp.status
	}
	attr, res := fs.GetAttr(resp.path)
	if res != OK {
		return nil, res
	}
	out.Attr = attr
	out.Ino = h.NodeId
	return out, OK
}

func getXAttr(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	out := new(GetXAttrOut)
	return out, OK
}

func openDir(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*OpenIn)
	fmt.Printf("FUSE_OPENDIR: %v\n", in)
	resp := c.openDir(h.NodeId)
	if resp.status != OK {
		return nil, resp.status
	}
	out := new(OpenOut)
	out.Fh = resp.fh
	return out, OK
}

func open(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*OpenIn)
	fmt.Printf("FUSE_OPEN: %v\n", in)
	resp := c.open(h.NodeId)
	if resp.status != OK {
		return nil, resp.status
	}
	out := new(OpenOut)
	out.Fh = resp.fh
	return out, OK
}

func readDir(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*ReadIn)
	fmt.Printf("FUSE_READDIR: %v\n", in)
	resp := c.getDirReader(h.NodeId, in.Fh)
	if resp.status != OK {
		return nil, resp.status
	}
	dirRespChan := make(chan *dirResponse, 1)
	fmt.Printf("Sending dir request, in.Offset: %v\n", in.Offset)
	resp.dirReq <- &dirRequest{false, h.NodeId, in.Offset, dirRespChan}
	fmt.Printf("receiving dir response\n")
	dirResp := <-dirRespChan
	fmt.Printf("received %v\n", dirResp)
	if dirResp.status != OK {
		return nil, dirResp.status
	}
	if dirResp.entries == nil {
		return nil, OK
	}

	buf := new(bytes.Buffer)
	off := in.Offset
	for _, entry := range dirResp.entries {
		off++
		dirent := new(Dirent)
		dirent.Off = off
		dirent.Ino = entry.nodeId
		dirent.NameLen = uint32(len(entry.name))
		dirent.Typ = (entry.mode & 0170000) >> 12
		err := binary.Write(buf, binary.LittleEndian, dirent)
		if err != nil {
			panic("Serialization of Dirent failed")
		}
		buf.Write([]byte(entry.name))
		buf.WriteByte(0)
		n := (len(entry.name) + 1) % 8 // padding
		if n != 0 {
			buf.Write(make([]byte, 8-n))
		}
	}
	out := buf.Bytes()
	return out, OK
}

func read(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*ReadIn)
	fmt.Printf("FUSE_READ: %v\n", in)
	resp := c.getFileReader(h.NodeId, in.Fh)
	if resp.status != OK {
		return nil, resp.status
	}
	fileRespChan := make(chan *fileResponse, 1)
	fmt.Printf("Sending file request, in.Offset: %v\n", in.Offset)
	resp.fileReq <- &fileRequest{ h.NodeId, in.Offset, in.Size, fileRespChan}
	fmt.Printf("receiving file response\n")
	fileResp := <-fileRespChan
	fmt.Printf("received %v\n", fileResp)
	if fileResp.status != OK {
		return nil, fileResp.status
	}
	return fileResp.data, OK
}

func flush(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*FlushIn)
	fmt.Printf("FUSE_FLUSH: %v\n", in)
	return nil, OK
}

func lookup(h *InHeader, r *bytes.Buffer, c *managerClient) (interface{}, Status) {
	filename := strings.TrimRight(string(r.Bytes()), "\x00")
	fmt.Printf("filename: %s\n", filename)
	resp := c.lookup(h.NodeId, filename)
	if resp.status != OK {
		return nil, resp.status
	}
	out := new(EntryOut)
	out.NodeId = resp.nodeId
	out.Attr = resp.attr
	out.AttrValid = 60
	out.EntryValid = 60
	return out, OK
}

func releaseDir(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*ReleaseIn)
	fmt.Printf("FUSE_RELEASEDIR: %v\n", in)
	resp := c.closeDir(h.NodeId, in.Fh)
	if resp.status != OK {
		return nil, resp.status
	}
	return nil, OK
}

func release(fs FileSystem, h *InHeader, ing interface{}, c *managerClient) (interface{}, Status) {
	in := ing.(*ReleaseIn)
	fmt.Printf("FUSE_RELEASE: %v\n", in)
	resp := c.closeFile(h.NodeId, in.Fh)
	if resp.status != OK {
		return nil, resp.status
	}
	return nil, OK
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
	lookupOp    = FileOp(4)
	getPathOp   = FileOp(5)
	openOp = FileOp(6)
	getFileHandleOp = FileOp(7)
	closeFileOp = FileOp(8)
)

type managerRequest struct {
	nodeId   uint64
	fh       uint64
	op       FileOp
	resp     chan *managerResponse
	filename string
}

type managerResponse struct {
	nodeId uint64
	fh     uint64
	dirReq chan *dirRequest
	fileReq chan *fileRequest
	status Status
	attr   Attr
	path   string
}

type dirEntry struct {
	nodeId uint64
	name   string
	mode   uint32
}

type dirRequest struct {
	isClose bool
	nodeId  uint64
	offset  uint64
	resp    chan *dirResponse
}

type dirResponse struct {
	entries []*dirEntry
	status  Status
}

type dirHandle struct {
	fh     uint64
	nodeId uint64
	req    chan *dirRequest
}

type fileRequest struct {
	nodeId uint64
	offset uint64
	size uint32
	resp chan *fileResponse
}

type fileResponse struct {
	data []byte
	status Status
}

type fileHandle struct {
	fh uint64
	nodeId uint64
	file File
	req chan *fileRequest
}

type manager struct {
	fs          FileSystem
	client      *managerClient
	dirHandles  map[uint64]*dirHandle
	fileHandles map[uint64]*fileHandle
	cnt         uint64
	nodes       map[uint64]string
	nodesByPath map[string]uint64
	nodeMax     uint64
}

func startManager(fs FileSystem, requests chan *managerRequest) {
	m := new(manager)
	m.fs = fs
	m.client = &managerClient{requests}
	m.dirHandles = make(map[uint64]*dirHandle)
	m.fileHandles = make(map[uint64]*fileHandle)
	m.nodes = make(map[uint64]string)
	m.nodes[0] = ""
	m.nodes[1] = "" // Root
	m.nodeMax = 1
	m.nodesByPath = make(map[string]uint64)
	m.nodesByPath[""] = 1
	go m.run(requests)
}

type managerClient struct {
	requests chan *managerRequest
}

func (c *managerClient) makeManagerRequest(nodeId uint64, fh uint64, op FileOp, filename string) (resp *managerResponse) {
	fmt.Printf("makeManagerRequest, nodeId = %d, fh = %d, op = %d, filename = %s\n", nodeId, fh, op, filename)
	req := &managerRequest{nodeId, fh, op, make(chan *managerResponse, 1), filename}
	c.requests <- req
	resp = <-req.resp
	fmt.Printf("makeManagerRequest, resp: %v\n", resp)
	return
}

func (c *managerClient) lookup(nodeId uint64, filename string) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, 0, lookupOp, filename)
}

func (c *managerClient) openDir(nodeId uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, 0, openDirOp, "")
}

func (c *managerClient) open(nodeId uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, 0, openOp, "")
}

func (c *managerClient) getDirReader(nodeId, fh uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, fh, getHandleOp, "")
}

func (c *managerClient) getFileReader(nodeId, fh uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, fh, getFileHandleOp, "")
}

func (c *managerClient) getPath(nodeId uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, 0, getPathOp, "")
}

func (c *managerClient) closeDir(nodeId, fh uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, fh, closeDirOp, "")
}

func (c *managerClient) closeFile(nodeId, fh uint64) (resp *managerResponse) {
	return c.makeManagerRequest(nodeId, fh, closeFileOp, "")
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
		case lookupOp:
			resp = m.lookup(req)
		case getPathOp:
			resp = m.getPath(req)
		case openOp:
			resp = m.open(req)
		case getFileHandleOp:
			resp = m.getFileHandle(req)
		case closeFileOp:
			resp = m.closeFile(req)
		default:
			panic(fmt.Sprintf("Unknown FileOp: %v", req.op))
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
	dir, ok := m.nodes[req.nodeId]
	if !ok {
		resp.status = ENOENT
		return
	}
	go readDirRoutine(dir, m.fs, m.client, h.req)
	resp.fh = h.fh
	return
}

func (m *manager) open(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	path, ok := m.nodes[req.nodeId]
	if !ok {
		resp.status = ENOENT
		return
	}
	var file File
	file, resp.status = m.fs.Open(path)
	if resp.status != OK {
		return
	}

	m.cnt++
	h := new(fileHandle)
	h.fh = m.cnt
	h.nodeId = req.nodeId
	h.file = file
	h.req = make(chan *fileRequest, 1)
	m.fileHandles[h.fh] = h
	go readFileRoutine(m.fs, m.client, h)
	resp.fh = h.fh
	return
}

func (m *manager) getHandle(req *managerRequest) (resp *managerResponse) {
	fmt.Printf("getHandle, fh: %v\n", req.fh)
	resp = new(managerResponse)
	h, ok := m.dirHandles[req.fh]
	if !ok {
		resp.status = ENOENT
		return
	}
	fmt.Printf("Handle found\n")
	resp.dirReq = h.req
	return
}

func (m *manager) getFileHandle(req *managerRequest) (resp *managerResponse) {
	fmt.Printf("getFileHandle, fh: %v\n", req.fh)
	resp = new(managerResponse)
	h, ok := m.fileHandles[req.fh]
	if !ok {
		resp.status = ENOENT
		return
	}
	fmt.Printf("File handle found\n")
	resp.fileReq = h.req
	return
}

func (m *manager) closeDir(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	h, ok := m.dirHandles[req.fh]
	if !ok {
		resp.status = ENOENT
		return
	}
	m.dirHandles[h.fh] = nil, false
	close(h.req)
	return
}

func (m *manager) closeFile(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	h, ok := m.fileHandles[req.fh]
	if !ok {
		resp.status = ENOENT
		return
	}
	file := h.file
	m.fileHandles[h.fh] = nil, false
	close(h.req)
	resp.status = file.Close()
	return
}

func (m *manager) lookup(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	parent, ok := m.nodes[req.nodeId]
	if !ok {
		resp.status = ENOENT
		return
	}
	attr, status := m.fs.GetAttr(path.Join(parent, req.filename))
	if status != OK {
		resp.status = status
	}
	resp.attr = attr
	fullPath := path.Clean(path.Join(parent, req.filename))
	nodeId, ok := m.nodesByPath[fullPath]
	if !ok {
		m.nodeMax++
		nodeId = m.nodeMax
		m.nodes[nodeId] = fullPath
		m.nodesByPath[fullPath] = nodeId
	}

	resp.nodeId = nodeId
	return
}

func (m *manager) getPath(req *managerRequest) (resp *managerResponse) {
	resp = new(managerResponse)
	path, ok := m.nodes[req.nodeId]
	if !ok {
		resp.status = ENOENT
		return
	}
	resp.path = path
	return
}

func readDirRoutine(dir string, fs FileSystem, c *managerClient, requests chan *dirRequest) {
	defer close(requests)
	dir = path.Clean(dir)
	names, status := fs.List(dir)
	i := uint64(0)
	for req := range requests {
		if status != OK {
			req.resp <- &dirResponse{nil, status}
			return
		}
		if req.offset != i {
			fmt.Printf("readDirRoutine: i = %v, changing offset to %v\n", i, req.offset)
			i = req.offset
		}
		if req.isClose {
			return
		}
		if i < uint64(len(names)) {
			entry := new(dirEntry)
			entry.name = names[i]
			lookupResp := c.lookup(req.nodeId, entry.name)
			if lookupResp.status != OK {
				req.resp <- &dirResponse{nil, lookupResp.status}
				return
			}
			entry.nodeId = lookupResp.nodeId
			entry.mode = lookupResp.attr.Mode
			req.resp <- &dirResponse{[]*dirEntry{entry}, OK}
			i++
		} else {
			req.resp <- &dirResponse{nil, OK}
		}
	}
}

func readFileRoutine(fs FileSystem, c *managerClient, h *fileHandle) {
	defer close(h.req)
	offset := uint64(0)
	for req := range h.req {
		data := make([]byte, req.size)
		n, err := h.file.ReadAt(data, int64(offset))
		if err != nil {
			req.resp <- &fileResponse { nil, EIO }
			continue
		}
		req.resp <- &fileResponse { data[0:n], OK }
	}
}
