// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"fmt"
	"log"
	"net"
	"reflect"
	"syscall"
	"unsafe"
)

type Ring struct {
	Num            int
	Desc           []VringDesc
	Avail          *VringAvail
	AvailRing      []uint16
	AvailUsedEvent *uint16
	Used           *VringUsed
	UsedRing       []VringUsedElement
	UsedAvailEvent *uint16

	LogGuestAddr []byte
	Flags        uint32
}

type VirtqInflight struct {
	Features      uint64
	Version       uint16
	DescNum       uint16
	LastBatchHead uint16
	UsedIdx       uint16

	Desc0 DescStateSplit // array.
}

type DescStateSplit struct {
	inflight uint8
	padding  [5]uint8
	next     uint16
	counter  uint64
}

type InflightDesc struct {
	index   uint16
	counter uint64
}

type Virtq struct {
	Vring Ring

	Inflight      *VirtqInflight
	InflightDescs []DescStateSplit
	ResubmitList  *InflightDesc

	ResubmitNum uint16

	Counter      uint64
	LastAvailIdx uint16

	ShadowAvailIdx uint16

	UsedIdx      uint16
	SignaledUsed uint16

	SignaledUsedValid bool
	Notification      bool

	inuse uint

	handler func(*Device, int)

	CallFD  int
	KickFD  int
	ErrFD   int
	Enable  uint
	Started bool

	Addr VhostVringAddr
}

func (vq *Virtq) availIdx() uint16 {
	// Weird, sideeffect?
	vq.ShadowAvailIdx = vq.Vring.Avail.Idx
	return vq.ShadowAvailIdx
}

func (vq *Virtq) queueEmpty() bool {
	// dev.broken
	// vq.vring == nil

	if vq.ShadowAvailIdx != vq.LastAvailIdx {
		return false
	}
	return vq.availIdx() == vq.LastAvailIdx
}

// Server implements the vhost-user protocol, which sets up the virtqs
// through a unix socket connection.
type Server struct {
	conn   *net.UnixConn
	device *Device

	Debug bool
}

func NewServer(c *net.UnixConn, d *Device) *Server {
	return &Server{conn: c, device: d}
}

func (s *Server) Serve() error {
	for {
		if err := s.oneRequest(); err != nil {
			return err
		}
	}
}

func (s *Server) getProtocolFeatures(rep *GetProtocolFeaturesReply) {
	rep.Mask = composeMask(s.device.GetProtocolFeatures())
}
func (s *Server) setProtocolFeatures(rep *SetProtocolFeaturesRequest) {
}

func (s *Server) getFeatures(rep *GetFeaturesReply) {
	rep.Mask = composeMask(s.device.GetFeatures())
}

func (s *Server) setFeatures(rep *SetFeaturesRequest) {
}

const hdrSize = int(unsafe.Sizeof(Header{}))

func (s *Server) oneRequest() error {
	var inBuf, oobBuf, outBuf [4096]byte

	// _ = flags is usually CLOEXEC.
	bufN, oobN, _, _, err := s.conn.ReadMsgUnix(inBuf[:hdrSize], oobBuf[:])
	oob := oobBuf[:oobN]
	if err != nil {
		return err
	}

	inHeader := (*Header)(unsafe.Pointer(&inBuf[0]))
	reqName := (reqNames[int(inHeader.Request)])

	var inFDs []int
	if len(oob) > 0 {
		scms, err := syscall.ParseSocketControlMessage(oob)
		if err != nil {
			return err
		}
		for _, scm := range scms {
			fds, err := syscall.ParseUnixRights(&scm)
			if err != nil {
				return err
			}
			inFDs = append(inFDs, fds...)

			// TODO make sockets non-blocking? See util/vhost-user-server.c l.179
		}
	}

	if inHeader.Size > 0 {
		bufN2, oobN2, flags2, addr2, err := s.conn.ReadMsgUnix(inBuf[hdrSize:hdrSize+int(inHeader.Size)], oobBuf[oobN:])
		if err != nil {
			return err
		}
		if bufN2 < int(inHeader.Size) {
			return fmt.Errorf("short read got %d want %d", bufN2, inHeader.Size)
		}
		oobN += oobN2
		bufN += bufN2

		if oobN2 > 0 {
			log.Printf("oob2 %q flags2 %x addr2 %x", oobBuf[oobN:oobN2+oobN], flags2, addr2)
		}
	}

	needReply := (inHeader.Flags & (0x1 << 3)) != 0
	inPayload := unsafe.Pointer(&inBuf[hdrSize])
	if s.Debug {
		inDebug := ""
		if f := decodeIn[inHeader.Request]; f != nil {
			// TODO - check payload size
			inDebug = fmt.Sprintf("%v", f(inPayload))
		} else if inHeader.Size > 0 {
			inDebug = fmt.Sprintf("payload %q (%d bytes)", inBuf[hdrSize:hdrSize+int(inHeader.Size)], inHeader.Size)
		}

		flagStr := ""
		if needReply {
			flagStr = "need_reply "
		}
		log.Printf("rx %-2d %s %s %sFDs %v", inHeader.Request, reqName, inDebug, flagStr, inFDs)
	}

	if c := inFDCount[inHeader.Request]; c != len(inFDs) {
		return fmt.Errorf("got %d fds for %s, want %d", len(inFDs), reqName, c)
	}

	var outHeader = (*Header)(unsafe.Pointer(&outBuf[0]))
	outPayloadPtr := unsafe.Pointer(&outBuf[hdrSize])
	inPayloadPtr := unsafe.Pointer(&inBuf[hdrSize])
	*outHeader = *inHeader
	outHeader.Flags |= 0x4 // reply

	var rep interface{}
	var deviceErr error
	switch inHeader.Request {
	case REQ_GET_FEATURES:
		r := (*GetFeaturesReply)(outPayloadPtr)
		s.getFeatures(r)
		rep = r
	case REQ_SET_FEATURES:
		req := (*SetFeaturesRequest)(inPayloadPtr)
		s.setFeatures(req)
	case REQ_GET_PROTOCOL_FEATURES:
		r := (*GetProtocolFeaturesReply)(outPayloadPtr)
		s.getProtocolFeatures(r)
		rep = r
	case REQ_SET_PROTOCOL_FEATURES:
		req := (*SetProtocolFeaturesRequest)(inPayloadPtr)
		s.setProtocolFeatures(req)

	case REQ_GET_QUEUE_NUM:
		r := (*U64Payload)(outPayloadPtr)
		r.Num = s.device.GetQueueNum()
		rep = r
	case REQ_GET_MAX_MEM_SLOTS:
		r := (*U64Payload)(outPayloadPtr)
		r.Num = s.device.GetMaxMemslots()
		rep = r
	case REQ_SET_BACKEND_REQ_FD:
		s.device.SetReqFD(inFDs[0])
	case REQ_SET_OWNER:
		// should pass in addr or something?
		s.device.SetOwner()
	case REQ_SET_VRING_CALL:
		req := (*U64Payload)(inPayloadPtr)
		s.device.SetVringCall(inFDs[0], req.Num)
	case REQ_SET_VRING_ERR:
		req := (*U64Payload)(inPayloadPtr)
		s.device.SetVringErr(inFDs[0], req.Num)
	case REQ_SET_VRING_KICK:
		req := (*U64Payload)(inPayloadPtr)
		deviceErr = s.device.SetVringKick(inFDs[0], req.Num)
	case REQ_ADD_MEM_REG:
		// req can also be u64 if in postcopy mode (sigh).
		req := (*VhostUserMemRegMsg)(inPayloadPtr)
		deviceErr = s.device.AddMemReg(inFDs[0], &req.Region)
	case REQ_SET_VRING_NUM:
		req := (*VhostVringState)(inPayloadPtr)
		s.device.SetVringNum(req)
	case REQ_SET_VRING_BASE:
		req := (*VhostVringState)(inPayloadPtr)
		s.device.SetVringBase(req)
	case REQ_SET_VRING_ENABLE:
		req := (*VhostVringState)(inPayloadPtr)
		s.device.SetVringEnable(req)
	case REQ_SET_VRING_ADDR:
		req := (*VhostVringAddr)(inPayloadPtr)
		deviceErr = s.device.SetVringAddr(req)
	case REQ_SET_LOG_BASE:
		req := (*VhostUserLog)(inPayloadPtr)
		s.device.SetLogBase(inFDs[0], req)
	default:
		log.Printf("unknown operation %d", inHeader.Request)
	}

	outPayloadSz := 0
	if needReply && rep == nil {
		r := (*U64Payload)(outPayloadPtr)
		if deviceErr != nil {
			log.Printf("request error: %v", deviceErr)
			r.Num = 1
		} else {
			r.Num = 0
		}
		rep = r

		// qemu doesn't like NEED_REPLY
		outHeader.Flags ^= (1 << 3)
	} else if deviceErr != nil {
		log.Printf("device error: %v", deviceErr)
	}

	var repBytes []byte
	if s.Debug {
		outDebug := "no reply"
		if rep != nil {
			outPayloadSz = int(reflect.ValueOf(rep).Elem().Type().Size())
			outHeader.Size = uint32(outPayloadSz)
			repBytes = outBuf[:hdrSize+outPayloadSz]

			if s, ok := rep.(fmt.Stringer); ok {
				outDebug = s.String()
			} else {
				outDebug = fmt.Sprintf("payload %q (%d bytes)", repBytes[hdrSize:], outPayloadSz)
			}
		}

		log.Printf("tx    %s %s", reqName, outDebug)
	}
	if len(repBytes) > 0 {
		if _, err := s.conn.Write(repBytes); err != nil {
			log.Printf("%v %T", err, err)
			return err
		}
	}
	return nil
}
