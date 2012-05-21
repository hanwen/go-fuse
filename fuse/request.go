package fuse

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

var sizeOfOutHeader = unsafe.Sizeof(raw.OutHeader{})
var zeroOutBuf [160]byte

func (req *request) Discard() {
	req.pool.FreeBuffer(req.flatData)
	req.pool.FreeBuffer(req.bufferPoolInputBuf)
}

type request struct {
	// the input, if obtained through bufferpool
	bufferPoolInputBuf []byte
	pool               BufferPool

	// If we have a small input, we quickly copy it to here, and
	// give back the large buffer to buffer pool.  The largest
	// input without data is 128 (setattr). 128 also fits small
	// requests with short filenames.
	smallInputBuf [128]byte
	
	inputBuf []byte

	// These split up inputBuf.
	inHeader  *raw.InHeader      // generic header
	inData    unsafe.Pointer // per op data
	arg       []byte         // flat data.
	
	filenames []string       // filename arguments

	// Unstructured data, a pointer to the relevant XxxxOut struct.
	outData  unsafe.Pointer
	status   Status
	flatData []byte

	// Space to keep header + structured data for what we send
	// back to the kernel.
	outBuf         [160]byte

	// Start timestamp for timing info.
	startNs    int64
	preWriteNs int64

	// All information pertaining to opcode of this request.
	handler *operationHandler
}

func (r *request) clear() {
	r.bufferPoolInputBuf = nil
	r.inputBuf = nil
	r.inHeader = nil
	r.inData = nil
	r.arg = nil
	r.filenames = nil
	r.outData = nil
	r.status = OK
	r.flatData = nil
	r.preWriteNs = 0
	r.startNs = 0
	r.handler = nil
}

func (r *request) InputDebug() string {
	val := " "
	if r.handler.DecodeIn != nil {
		val = fmt.Sprintf(" data: %v ", r.handler.DecodeIn(r.inData))
	}

	names := ""
	if r.filenames != nil {
		names = fmt.Sprintf("names: %v", r.filenames)
	}

	if len(r.arg) > 0 {
		names += fmt.Sprintf(" %d bytes", len(r.arg))
	}

	return fmt.Sprintf("Dispatch: %s, NodeId: %v.%v%v",
		operationName(r.inHeader.Opcode), r.inHeader.NodeId, val, names)
}

func (r *request) OutputDebug() string {
	var val interface{}
	if r.handler.DecodeOut != nil && r.outData != nil {
		val = r.handler.DecodeOut(r.outData)
	}

	dataStr := ""
	if val != nil {
		dataStr = fmt.Sprintf("%v", val)
	}

	max := 1024
	if len(dataStr) > max {
		dataStr = dataStr[:max] + fmt.Sprintf(" ...trimmed")
	}

	flatStr := ""
	if len(r.flatData) > 0 {
		if r.handler.FileNameOut {
			s := strings.TrimRight(string(r.flatData), "\x00")
			flatStr = fmt.Sprintf(" %q", s)
		} else {
			flatStr = fmt.Sprintf(" %d bytes data\n", len(r.flatData))
		}
	}

	return fmt.Sprintf("Serialize: %s code: %v value: %v%v",
		operationName(r.inHeader.Opcode), r.status, dataStr, flatStr)
}

// setInput returns true if it takes ownership of the argument, false if not.
func (r *request) setInput(input []byte) bool {
	if len(input) < len(r.smallInputBuf) {
		copy(r.smallInputBuf[:], input)
		r.inputBuf = r.smallInputBuf[:len(input)]
		return false
	} 
	r.inputBuf = input
	r.bufferPoolInputBuf = input
	
	return true
}

func (r *request) parse() {
	inHSize := int(unsafe.Sizeof(raw.InHeader{}))
	if len(r.inputBuf) < inHSize {
		log.Printf("Short read for input header: %v", r.inputBuf)
		return
	}

	r.inHeader = (*raw.InHeader)(unsafe.Pointer(&r.inputBuf[0]))
	r.arg = r.inputBuf[inHSize:]

	r.handler = getHandler(r.inHeader.Opcode)
	if r.handler == nil {
		log.Printf("Unknown opcode %d", r.inHeader.Opcode)
		r.status = ENOSYS
		return
	}

	if len(r.arg) < int(r.handler.InputSize) {
		log.Printf("Short read for %v: %v", operationName(r.inHeader.Opcode), r.arg)
		r.status = EIO
		return
	}

	if r.handler.InputSize > 0 {
		r.inData = unsafe.Pointer(&r.arg[0])
		r.arg = r.arg[r.handler.InputSize:]
	}

	count := r.handler.FileNames
	if count > 0 {
		if count == 1 {
			r.filenames = []string{string(r.arg[:len(r.arg)-1])}
		} else {
			names := bytes.SplitN(r.arg[:len(r.arg)-1], []byte{0}, count)
			r.filenames = make([]string, len(names))
			for i, n := range names {
				r.filenames[i] = string(n)
			}
			if len(names) != count {
				log.Println("filename argument mismatch", names, count)
				r.status = EIO
			}
		}
	}

	r.outBuf = zeroOutBuf
	r.outData = unsafe.Pointer(&r.outBuf[sizeOfOutHeader])
}

func (r *request) serialize() (header []byte, data []byte) {
	dataLength := r.handler.OutputSize
	if r.outData == nil || r.status > OK {
		dataLength = 0
	}

	sizeOfOutHeader := unsafe.Sizeof(raw.OutHeader{})
	header = r.outBuf[:sizeOfOutHeader+dataLength]
	o := (*raw.OutHeader)(unsafe.Pointer(&header[0]))
	o.Unique = r.inHeader.Unique
	o.Status = int32(-r.status)
	o.Length = uint32(
		int(sizeOfOutHeader) + int(dataLength) + int(len(r.flatData)))

	copy(header[sizeOfOutHeader:], asSlice(r.outData, dataLength))
	return header, r.flatData
}

