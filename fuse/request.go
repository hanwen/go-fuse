package fuse

import (
	"fmt"
	"bytes"
	"log"
	"unsafe"
)

type request struct {
	inputBuf []byte

	// These split up inputBuf.
	inHeader *InHeader      // generic header
	inData   unsafe.Pointer // per op data
	arg      []byte         // flat data.

	// Unstructured data, a pointer to the relevant XxxxOut struct.
	outData  unsafe.Pointer
	status   Status
	flatData []byte

	// Header + structured data for what we send back to the kernel.
	// May be followed by flatData.
	outHeaderBytes []byte

	// Start timestamp for timing info.
	startNs    int64
	preWriteNs int64

	// All information pertaining to opcode of this request.
	handler *operationHandler
}

func (me *request) filename() string {
	return string(me.arg[:len(me.arg)-1])
}

func (me *request) filenames(count int) []string {
	names := bytes.Split(me.arg[:len(me.arg)-1], []byte{0}, count)
	nameStrings := make([]string, len(names))
	for i, n := range names {
		nameStrings[i] = string(n)
	}
	return nameStrings
}

func (me *request) InputDebug() string {
	val := " "
	if me.handler.DecodeIn != nil {
		val = fmt.Sprintf(" data: %v ", me.handler.DecodeIn(me.inData))
	}

	names := ""
	if me.handler.FileNames > 0 {
		names = fmt.Sprintf("names: %v", me.filenames(me.handler.FileNames))
	}

	return fmt.Sprintf("Dispatch: %v, NodeId: %v.%v%v",
		me.inHeader.opcode, me.inHeader.NodeId, val, names)
}

func (me *request) OutputDebug() string {
	var val interface{}
	if me.handler.DecodeOut != nil {
		val = me.handler.DecodeOut(me.outData)
	}

	dataStr := ""
	if val != nil {
		dataStr = fmt.Sprintf("%v", val)
	}

	max := 1024
	if len(dataStr) > max {
		dataStr = dataStr[:max] + fmt.Sprintf(" ...trimmed (response size %d)", len(me.outHeaderBytes))
	}

	flatStr := ""
	if len(me.flatData) > 0 {
		flatStr = fmt.Sprintf(" %d bytes data\n", len(me.flatData))
	}

	return fmt.Sprintf("Serialize: %v code: %v value: %v%v",
		me.inHeader.opcode, me.status, dataStr, flatStr)
}

func (req *request) parse() {
	inHSize := unsafe.Sizeof(InHeader{})
	if len(req.inputBuf) < inHSize {
		log.Printf("Short read for input header: %v", req.inputBuf)
		return
	}

	req.inHeader = (*InHeader)(unsafe.Pointer(&req.inputBuf[0]))
	req.arg = req.inputBuf[inHSize:]

	req.handler = getHandler(req.inHeader.opcode)
	if req.handler == nil || req.handler.Func == nil {
		msg := "Unimplemented"
		if req.handler == nil {
			msg = "Unknown"
		}
		log.Printf("%s opcode %v", msg, req.inHeader.opcode)
		req.status = ENOSYS
		return
	}

	if len(req.arg) < req.handler.InputSize {
		log.Printf("Short read for %v: %v", req.inHeader.opcode, req.arg)
		req.status = EIO
		return
	}

	if req.handler.InputSize > 0 {
		req.inData = unsafe.Pointer(&req.arg[0])
		req.arg = req.arg[req.handler.InputSize:]
	}
}

func (req *request) serialize() {
	dataLength := req.handler.OutputSize
	if req.outData == nil || req.status != OK {
		dataLength = 0
	}

	sizeOfOutHeader := unsafe.Sizeof(OutHeader{})

	req.outHeaderBytes = make([]byte, sizeOfOutHeader+dataLength)
	outHeader := (*OutHeader)(unsafe.Pointer(&req.outHeaderBytes[0]))
	outHeader.Unique = req.inHeader.Unique
	outHeader.Status = -req.status
	outHeader.Length = uint32(sizeOfOutHeader + dataLength + len(req.flatData))

	copy(req.outHeaderBytes[sizeOfOutHeader:], asSlice(req.outData, dataLength))
}
