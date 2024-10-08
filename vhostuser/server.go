package vhostuser

import (
	"fmt"
	"log"
	"net"
	"reflect"
	"sort"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type DeviceRegion struct {
	VhostUserMemoryRegion

	//	MmapAddr uint64
	Data []byte
}

func (r *DeviceRegion) String() string {
	return r.VhostUserMemoryRegion.String()
}

func (r *DeviceRegion) containsGuestAddr(guestAddr uint64) bool {
	return guestAddr >= r.GuestPhysAddr && guestAddr < r.GuestPhysAddr+r.MemorySize
}

func (r *DeviceRegion) FromDriverAddr(driverAddr uint64) unsafe.Pointer {
	if driverAddr < r.VhostUserMemoryRegion.DriverAddr || driverAddr >= r.DriverAddr+r.MemorySize {
		return nil
	}

	return unsafe.Pointer(&r.Data[driverAddr-r.DriverAddr+r.MmapOffset])
}

type FSDevice struct {
	reqFD int

	// vring is the same as virtq?
	vqs []Virtq

	// sorted by GuestPhysAddr
	regions []DeviceRegion

	handle func(*VirtqElem)
}

func NewFSDevice() *FSDevice {
	d := &FSDevice{
		vqs: make([]Virtq, 2),
	}
	for i := range d.vqs {
		d.vqs[i].Notification = true
	}
	return d
}

type Ring struct {
	Num            int
	Desc           []VringDesc
	Avail          *VringAvail
	AvailRing      []uint16
	AvailUsedEvent *uint16
	Used           *VringUsed
	UsedRing       []VringUsedElement
	UsedAvailEvent *uint16

	LogGuestAddr uint64
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

	Inflight VirtqInflight

	ResubmitList *InflightDesc

	ResubmitNum uint16

	Counter      uint64
	LastAvailIdx uint16

	ShadowAvailIdx uint16

	UsedIdx      uint16
	SignaledUsed uint16

	SignaledUsedValid bool
	Notification      bool

	inuse uint

	handler func(*FSDevice, int)

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

func (d *FSDevice) MapRing(vq *Virtq) error {
	if d := d.FromDriverAddr(vq.Addr.DescUserAddr); d == nil {
		return fmt.Errorf("could not map DescUserAddr %x", vq.Addr.DescUserAddr)
	} else {
		vq.Vring.Desc = unsafe.Slice((*VringDesc)(d), vq.Vring.Num)
	}
	if d := d.FromDriverAddr(vq.Addr.UsedUserAddr); d == nil {
		return fmt.Errorf("could not map UsedUserAddr %x",
			vq.Addr.UsedUserAddr)
	} else {
		vq.Vring.Used = (*VringUsed)(d)
		vq.Vring.UsedRing = unsafe.Slice(&vq.Vring.Used.Ring0, vq.Vring.Num)
		//if (vu_has_feature(dev, VIRTIO_RING_F_EVENT_IDX)) {
		vq.Vring.UsedAvailEvent = (*uint16)(unsafe.Pointer(&unsafe.Slice(&vq.Vring.Used.Ring0, vq.Vring.Num+1)[vq.Vring.Num]))

	}

	if d := d.FromDriverAddr(vq.Addr.AvailUserAddr); d == nil {
		return fmt.Errorf("could not map AvailUserAddr %x",
			vq.Addr.AvailUserAddr)
	} else {
		vq.Vring.Avail = (*VringAvail)(d)
		vq.Vring.AvailRing = unsafe.Slice(&vq.Vring.Avail.Ring0, vq.Vring.Num)
		//if (vu_has_feature(dev, VIRTIO_RING_F_EVENT_IDX)) {
		vq.Vring.AvailUsedEvent = &unsafe.Slice(&vq.Vring.Avail.Ring0, vq.Vring.Num+1)[vq.Vring.Num]
	}
	return nil
}

func (d *FSDevice) FromDriverAddr(driverAddr uint64) unsafe.Pointer {
	for _, r := range d.regions {
		d := r.FromDriverAddr(driverAddr)
		if d != nil {
			return d
		}
	}
	return nil
}

func (d *FSDevice) FromGuestAddr(guestAddr uint64, sz uint64) []byte {
	idx := d.findRegionByGuestAddr(guestAddr)
	r := d.regions[idx]
	if !r.containsGuestAddr(guestAddr) {
		return nil
	}

	seg := r.Data[guestAddr-r.GuestPhysAddr:]
	if len(seg) > int(sz) {
		seg = seg[:sz]
	}
	return seg
}

func (d *FSDevice) SetVringAddr(addr *VhostVringAddr) error {
	vq := &d.vqs[addr.Index]
	vq.Addr = *addr
	vq.Vring.Flags = uint32(addr.Flags) // bitsize?
	vq.Vring.LogGuestAddr = addr.LogGuestAddr

	if err := d.MapRing(vq); err != nil {
		return err
	}

	vq.UsedIdx = vq.Vring.Used.Idx // LE16toH
	if vq.LastAvailIdx != vq.UsedIdx {
		resume := true // device->processed_in_order()
		if resume {
			vq.ShadowAvailIdx = vq.UsedIdx
			vq.LastAvailIdx = vq.UsedIdx
		}
	}

	return nil
}

func (d *FSDevice) SetVringNum(state *VhostVringState) {
	d.vqs[state.Index].Vring.Num = int(state.Num)
}

func (d *FSDevice) SetVringBase(state *VhostVringState) {
	p := &d.vqs[state.Index]
	p.ShadowAvailIdx = uint16(state.Num)
	p.LastAvailIdx = uint16(state.Num)
}
func (d *FSDevice) SetVringEnable(state *VhostVringState) {
	p := &d.vqs[state.Index]
	p.Enable = uint(state.Num)
	d.kickMe(state.Index)
}

func (d *FSDevice) kickMe(idx uint32) {
	vq := &d.vqs[idx]

	// todo: mimick vu_queue_pop(VuDev *dev, VuVirtq *vq, size_t sz)
	go func() {
		for {
			var id [8]byte
			_, err := syscall.Read(vq.KickFD, id[:])

			data, err := d.popQueue(vq)
			if err != nil {
				log.Printf("popq: %v", err)
				continue
			}
			if data == nil {
				log.Printf("queue was empty")
				continue
			}
			log.Printf("popQ: in %q out %q",
				data.in,
				data.out)

			// should pass on vq as well?
			if d.handle != nil {
				d.handle(data)
			} else {
				log.Printf("no handler defined")
			}
		}
	}()
}

type VirtqElem struct {
	index uint
	in    [][]byte
	out   [][]byte
}

func (d *FSDevice) dumpRegions() {
	for i, r := range d.regions {
		log.Printf("region %d: %v", i, &r)
	}
}

func (d *FSDevice) popQueue(vq *Virtq) (*VirtqElem, error) {

	/* TODO: unlikely conditions */

	// dev->broken?
	// vq.vring.avail == 0
	if vq.ResubmitList != nil && vq.ResubmitNum > 0 {
		return nil, fmt.Errorf("resubmit")
	}

	if vq.queueEmpty() {
		return nil, nil
	}

	if int(vq.inuse) >= vq.Vring.Num {
		return nil, fmt.Errorf("virtq size exceeded")
	}

	// todo RMB read barrier.

	idx := int(vq.LastAvailIdx) % vq.Vring.Num
	result := VirtqElem{
		index: uint(idx),
	}

	vq.LastAvailIdx++
	head := vq.Vring.AvailRing[idx]
	if int(head) > vq.Vring.Num {
		log.Panicf("silly avail %d %d", head, vq.Vring.Num)
	}
	log.Printf("head %d", head)
	if vq.Vring.UsedAvailEvent != nil {
		*vq.Vring.UsedAvailEvent = vq.LastAvailIdx
	}

	// vu_queue_map_desc
	descArray := vq.Vring.Desc
	desc := descArray[head]
	log.Printf("desc %v", &desc)
	d.dumpRegions()
	if desc.Flags&VRING_DESC_F_INDIRECT != 0 {
		eltSize := unsafe.Sizeof(VringDesc{})
		if (desc.Len % uint32(eltSize)) != 0 {
			return nil, fmt.Errorf("modulo size")
		}

		indirectAsBytes := d.FromGuestAddr(desc.Addr, uint64(desc.Len))
		if indirectAsBytes == nil {
			return nil, fmt.Errorf("OOB read %x %#v", desc.Addr, d.regions)
		}
		if len(indirectAsBytes) != int(desc.Len) {
			return nil, fmt.Errorf("partial read indirect desc")
		}
		n := desc.Len / uint32(eltSize)
		descArray = unsafe.Slice((*VringDesc)(unsafe.Pointer(&indirectAsBytes[0])), n)
		desc = descArray[0]

		log.Printf("desc array: %v", descArray)
	}

	for {
		iov := d.virtqMapDesc(desc.Addr, desc.Len)
		log.Printf("got iov %q %d", iov, len(iov))
		if desc.Flags&VRING_DESC_F_WRITE != 0 {
			// virtqueue_map_desc
			result.in = append(result.in, iov...)
		} else {
			result.out = append(result.out, iov...)
		}
		//

		if desc.Flags&VRING_DESC_F_NEXT == 0 {
			break
		}

		head = desc.Next
		// barrier

		// todo: check max

		desc = descArray[head]
	}

	return &result, nil
}

// take VIRTQUEUE_MAX_SIZE ?
func (d *FSDevice) virtqMapDesc(physAddr uint64, sz uint32) [][]byte {
	var result [][]byte

	for sz > 0 {
		d := d.FromGuestAddr(physAddr, uint64(sz))
		result = append(result, d)
		sz -= uint32(len(d))
		physAddr += uint64(len(d))
	}

	return result
}

func (d *FSDevice) findRegionByGuestAddr(guestAddr uint64) int {
	return sort.Search(len(d.regions),
		func(i int) bool {
			return guestAddr < d.regions[i].GuestPhysAddr+d.regions[i].MemorySize
		})
}

func (d *FSDevice) AddMemReg(fd int, reg *VhostUserMemoryRegion) error {
	if len(d.regions) == int(d.GetMaxMemslots()) {
		return fmt.Errorf("hot add memory")
	}

	idx := d.findRegionByGuestAddr(reg.GuestPhysAddr)
	if hps := GetFDHugepagesize(fd); hps != 0 {
		return fmt.Errorf("huge pages")
	}

	data, err := syscall.Mmap(fd, int64(reg.MmapOffset), int(reg.MemorySize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_NORESERVE)
	if err != nil {
		return err
	}
	syscall.Madvise(data, unix.MADV_DONTDUMP)

	d.regions = append(d.regions, DeviceRegion{})
	copy(d.regions[idx+1:], d.regions[idx:])
	d.regions[idx] = DeviceRegion{
		VhostUserMemoryRegion: VhostUserMemoryRegion{
			GuestPhysAddr: reg.GuestPhysAddr,
			MemorySize:    reg.MemorySize,
			DriverAddr:    reg.DriverAddr,
			MmapOffset:    0, // input holds the offset into the fd.
		},
		Data: data,
	}
	return nil
}

func (d *FSDevice) SetVringKick(fd int, index uint64) error {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}
	old := d.vqs[index].KickFD
	if old != 0 {
		syscall.Close(old)
	}
	d.vqs[index].KickFD = fd

	return syscall.SetNonblock(fd, false)
}

// todo consolidate
func (d *FSDevice) SetVringErr(fd int, index uint64) {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}

	if old := d.vqs[index].ErrFD; old != 0 {
		syscall.Close(old)
	}

	d.vqs[index].ErrFD = fd
}

func (d *FSDevice) SetVringCall(fd int, index uint64) {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}
	if old := d.vqs[index].CallFD; old != 0 {
		syscall.Close(old)
	}
	d.vqs[index].CallFD = fd
}

func (d *FSDevice) SetOwner() {

}

func (d *FSDevice) SetReqFD(fd int) {
	d.reqFD = fd
}

const MAX_MEM_SLOTS = 509

func (d *FSDevice) GetMaxMemslots() uint64 {
	return MAX_MEM_SLOTS
}

func (d *FSDevice) GetQueueNum() uint64 {
	return uint64(len(d.vqs))
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

// not supporting VHOST_USER_PROTOCOL_F_PAGEFAULT, so no support for
// postcopy listening.
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
	device *FSDevice
}

type empty struct{}

func NewServer(c *net.UnixConn, d *FSDevice) *Server {
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

	inPayload := unsafe.Pointer(&inBuf[hdrSize])
	inDebug := ""
	if f := decodeIn[inHeader.Request]; f != nil {
		// TODO - check payload size
		inDebug = fmt.Sprintf("%v", f(inPayload))
	} else if inHeader.Size > 0 {
		inDebug = fmt.Sprintf("payload %q (%d bytes)", inBuf[hdrSize:hdrSize+int(inHeader.Size)], inHeader.Size)
	}

	needReply := (inHeader.Flags & (0x1 << 3)) != 0
	flagStr := ""
	if needReply {
		flagStr = "need_reply "
	}
	log.Printf("rx %-2d %s %s %sFDs %v", inHeader.Request, reqName, inDebug, flagStr, inFDs)

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

	if len(repBytes) > 0 {
		if _, err := s.conn.Write(repBytes); err != nil {
			log.Printf("%v %T", err, err)
			return err
		}
	}
	return nil
}

const HUGETLBFS_MAGIC = 0x958458f6

func GetFDHugepagesize(fd int) int {
	var fs syscall.Statfs_t
	var err error
	for {
		err = syscall.Fstatfs(fd, &fs)
		if err != syscall.EINTR {
			break
		}
	}

	if err == nil && fs.Type == HUGETLBFS_MAGIC {
		return int(fs.Bsize)
	}
	return 0
}
