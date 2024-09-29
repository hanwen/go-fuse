package vhostuser

import (
	"fmt"
	"log"
	"net"
	"reflect"
	"unsafe"
)

type Device interface {
	GetFeatures() []int
	SetFeatures([]int)
	GetProtocolFeatures() []int
	SetProtocolFeatures([]int)
}

type FSDevice struct {
}

func (h *FSDevice) GetFeatures() []int {
	return []int{
		//"\0\0\0p\1\0\0\0"
		RING_F_INDIRECT_DESC,
		RING_F_EVENT_IDX,
		F_PROTOCOL_FEATURES,
		F_VERSION_1,
	}
}

func (h *FSDevice) SetFeatures(fs []int) {

}

func (h *FSDevice) SetProtocolFeatures([]int) {

}

func (h *FSDevice) GetProtocolFeatures() []int {
	// ")\204\0\0\0\0\0\0"
	// x29 x84
	return []int{
		PROTOCOL_F_MQ,
		PROTOCOL_F_REPLY_ACK,
		PROTOCOL_F_BACKEND_REQ,
		PROTOCOL_F_BACKEND_SEND_FD,
		PROTOCOL_F_CONFIGURE_MEM_SLOTS,
	}
}

type Server struct {
	conn   *net.UnixConn
	device Device
}

type empty struct{}

func NewServer(c *net.UnixConn, d Device) *Server {
	return &Server{conn: c, device: d}
}

func (s *Server) Serve() error {
	for {
		if err := s.oneRequest(); err != nil {
			return err
		}
	}
}
func composeMask(fs []int) uint64 {
	var mask uint64
	for _, f := range fs {
		mask |= (uint64(0x1) << f)
	}
	return mask
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
	bufN, oobN, flags, addr, err := s.conn.ReadMsgUnix(inBuf[:], oobBuf[:])
	oob := oobBuf[:oobN]
	if err != nil {
		return err
	}
	inHeader := (*Header)(unsafe.Pointer(&inBuf[0]))
	reqName := (reqNames[int(inHeader.Request)])

	payloadSz := bufN - hdrSize
	for payloadSz < int(inHeader.Size) {
		n, err := s.conn.Read(inBuf[bufN:])
		if err != nil {
			return err
		}
		payloadSz += n
		bufN += n
	}

	if payloadSz > int(inHeader.Size) {
		return fmt.Errorf("read %d bytes, should be %d", payloadSz, inHeader.Size)
	}

	inPayload := unsafe.Pointer(&inBuf[hdrSize])
	inDebug := ""
	if f := decodeIn[inHeader.Request]; f != nil {
		inDebug = fmt.Sprintf("%v", f(inPayload))
	}

	log.Printf("rx %s %s flags %x OOB %q addr %x", reqName, inDebug, flags, oob, addr)

	var outHeader = (*Header)(unsafe.Pointer(&outBuf[0]))
	outPayloadPtr := unsafe.Pointer(&outBuf[hdrSize])
	inPayloadPtr := unsafe.Pointer(&inBuf[hdrSize])
	*outHeader = *inHeader
	outHeader.Flags |= 0x4 // reply

	var rep interface{}
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
	default:
		log.Printf("unknown operation %d", inHeader.Request)
	}

	outPayloadSz := 0
	if rep != nil {
		outDebug := ""
		if s, ok := rep.(fmt.Stringer); ok {
			outDebug = s.String()
		}

		log.Printf("tx %s %s", reqName, outDebug)
		outPayloadSz = int(reflect.ValueOf(rep).Elem().Type().Size())
	}
	outHeader.Size = uint32(outPayloadSz)
	repBytes := outBuf[:hdrSize+outPayloadSz]
	log.Printf("replying %q", repBytes)
	if _, err := s.conn.Write(repBytes); err != nil {
		log.Printf("%v %T", err, err)
		return err
	}

	return nil
}
