// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"
	"unsafe"
)

var sizeOfOutHeader = unsafe.Sizeof(OutHeader{})
var zeroOutBuf [outputHeaderSize]byte

type request struct {
	inflightIndex int

	cancel chan struct{}

	// written under Server.reqMu
	interrupted bool

	inputBuf []byte

	// These split up inputBuf.
	arg []byte // argument to the operation, eg. data to write.

	filenames []string // filename arguments

	// Output data.
	status   Status
	flatData []byte
	fdData   *readResultFd

	// In case of read, keep read result here so we can call
	// Done() on it.
	readResult ReadResult

	// Start timestamp for timing info.
	startTime time.Time

	// All information pertaining to opcode of this request.
	handler *operationHandler

	// Request storage. For large inputs and outputs, use data
	// obtained through bufferpool.
	bufferPoolInputBuf  []byte
	bufferPoolOutputBuf []byte

	// For small pieces of data, we use the following inlines
	// arrays:
	//
	// Output header and structured data.
	outBuf [outputHeaderSize]byte

	// Input, if small enough to fit here.
	smallInputBuf [128]byte
}

func (r *request) inHeader() *InHeader {
	return (*InHeader)(r.inData())
}

func (r *request) outHeader() *OutHeader {
	return (*OutHeader)(unsafe.Pointer(&r.outBuf[0]))
}

func (r *request) clear() {
	r.inputBuf = nil
	r.arg = nil
	r.filenames = nil
	r.status = OK
	r.flatData = nil
	r.fdData = nil
	r.startTime = time.Time{}
	r.handler = nil
	r.readResult = nil
}

func asType(ptr unsafe.Pointer, typ interface{}) interface{} {
	return reflect.NewAt(reflect.ValueOf(typ).Type(), ptr).Interface()
}

func (r *request) InputDebug() string {
	val := ""
	if r.handler != nil && r.handler.InType != nil {
		val = Print(asType(r.inData(), r.handler.InType))
	}

	names := ""
	if r.filenames != nil {
		names = fmt.Sprintf("%q", r.filenames)
	}

	if l := len(r.arg); l > 0 {
		data := ""
		if len(r.filenames) == 0 {
			dots := ""
			if l > 8 {
				l = 8
				dots = "..."
			}

			data = fmt.Sprintf("%q%s", r.arg[:l], dots)
		}

		names += fmt.Sprintf("%s %db", data, len(r.arg))
	}

	return fmt.Sprintf("rx %d: %s n%d %s%s p%d",
		r.inHeader().Unique, operationName(r.inHeader().Opcode), r.inHeader().NodeId,
		val, names, r.inHeader().Caller.Pid)
}

func (r *request) OutputDebug() string {
	var dataStr string
	if r.handler != nil && r.handler.OutType != nil && r.outHeader().Length > uint32(sizeOfOutHeader) {
		dataStr = Print(asType(r.outData(), r.handler.OutType))
	}

	max := 1024
	if len(dataStr) > max {
		dataStr = dataStr[:max] + " ...trimmed"
	}

	flatStr := ""
	if r.flatDataSize() > 0 {
		if r.handler != nil && r.handler.FileNameOut {
			s := strings.TrimRight(string(r.flatData), "\x00")
			flatStr = fmt.Sprintf(" %q", s)
		} else {
			spl := ""
			if r.fdData != nil {
				spl = " (fd data)"
			} else {
				l := len(r.flatData)
				s := ""
				if l > 8 {
					l = 8
					s = "..."
				}
				spl = fmt.Sprintf(" %q%s", r.flatData[:l], s)
			}
			flatStr = fmt.Sprintf(" %db data%s", r.flatDataSize(), spl)
		}
	}

	extraStr := dataStr + flatStr
	if extraStr != "" {
		extraStr = ", " + extraStr
	}
	return fmt.Sprintf("tx %d:     %v%s",
		r.inHeader().Unique, r.status, extraStr)
}

// setInput returns true if it takes ownership of the argument, false if not.
func (r *request) setInput(input []byte) bool {
	if len(input) < len(r.smallInputBuf) {
		copy(r.smallInputBuf[:], input)
		r.inputBuf = r.smallInputBuf[:len(input)]
		return false
	}
	r.inputBuf = input
	r.bufferPoolInputBuf = input[:cap(input)]

	return true
}

func (r *request) inData() unsafe.Pointer {
	return unsafe.Pointer(&r.inputBuf[0])
}

func (r *request) parse(kernelSettings *InitIn) {
	r.handler = getHandler(r.inHeader().Opcode)
	if r.handler == nil {
		log.Printf("Unknown opcode %d", r.inHeader().Opcode)
		r.status = ENOSYS
		return
	}

	inSz := int(r.handler.InputSize)
	if r.inHeader().Opcode == _OP_RENAME && kernelSettings.supportsRenameSwap() {
		inSz = int(unsafe.Sizeof(RenameIn{}))
	}
	if r.inHeader().Opcode == _OP_INIT && inSz > len(r.arg) {
		// Minor version 36 extended the size of InitIn struct
		inSz = len(r.inputBuf)
	}
	if len(r.inputBuf) < inSz {
		log.Printf("Short read for %v: %q", operationName(r.inHeader().Opcode), r.inputBuf)
		r.status = EIO
		return
	}

	if r.handler.InputSize > 0 {
		r.arg = r.inputBuf[inSz:]
	} else {
		r.arg = r.inputBuf[unsafe.Sizeof(InHeader{}):]
	}

	count := r.handler.FileNames
	if count > 0 {
		if count == 1 && r.inHeader().Opcode == _OP_SETXATTR {
			// SETXATTR is special: the only opcode with a file name AND a
			// binary argument.
			splits := bytes.SplitN(r.arg, []byte{0}, 2)
			r.filenames = []string{string(splits[0])}
		} else if count == 1 {
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

	copy(r.outBuf[:r.handler.OutputSize+sizeOfOutHeader],
		zeroOutBuf[:r.handler.OutputSize+sizeOfOutHeader])

}

func (r *request) outData() unsafe.Pointer {
	return unsafe.Pointer(&r.outBuf[sizeOfOutHeader])
}

// serializeHeader serializes the response header. The header points
// to an internal buffer of the receiver.
func (r *request) serializeHeader(flatDataSize int) (header []byte) {
	var dataLength uintptr

	if r.handler != nil {
		dataLength = r.handler.OutputSize
	}
	if r.status > OK {
		// only do this for positive status; negative status
		// is used for notification.
		dataLength = 0
	}

	// [GET|LIST]XATTR is two opcodes in one: get/list xattr size (return
	// structured GetXAttrOut, no flat data) and get/list xattr data
	// (return no structured data, but only flat data)
	if r.inHeader().Opcode == _OP_GETXATTR || r.inHeader().Opcode == _OP_LISTXATTR {
		if (*GetXAttrIn)(r.inData()).Size != 0 {
			dataLength = 0
		}
	}

	header = r.outBuf[:sizeOfOutHeader+dataLength]

	o := r.outHeader()
	o.Unique = r.inHeader().Unique
	o.Status = int32(-r.status)
	o.Length = uint32(
		int(sizeOfOutHeader) + int(dataLength) + flatDataSize)
	return header
}

func (r *request) flatDataSize() int {
	if r.fdData != nil {
		return r.fdData.Size()
	}
	return len(r.flatData)
}
