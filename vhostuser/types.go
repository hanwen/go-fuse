// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

// protocol features vhost-user.h
const (
	PROTOCOL_F_MQ             = 0
	PROTOCOL_F_LOG_SHMFD      = 1
	PROTOCOL_F_RARP           = 2
	PROTOCOL_F_REPLY_ACK      = 3
	PROTOCOL_F_NET_MTU        = 4
	PROTOCOL_F_BACKEND_REQ    = 5
	PROTOCOL_F_CROSS_ENDIAN   = 6
	PROTOCOL_F_CRYPTO_SESSION = 7
	PROTOCOL_F_PAGEFAULT      = 8
	PROTOCOL_F_CONFIG         = 9
	// aka. VHOST_USER_PROTOCOL_F_SLAVE_SEND_FD =10
	PROTOCOL_F_BACKEND_SEND_FD      = 10
	PROTOCOL_F_HOST_NOTIFIER        = 11
	PROTOCOL_F_INFLIGHT_SHMFD       = 12
	PROTOCOL_F_RESET_DEVICE         = 13
	PROTOCOL_F_INBAND_NOTIFICATIONS = 14
	PROTOCOL_F_CONFIGURE_MEM_SLOTS  = 15
	PROTOCOL_F_STATUS               = 16
	/* Feature 17 reserved for PROTOCOL_F_XEN_MMAP. */
	PROTOCOL_F_SHARED_OBJECT = 18
	PROTOCOL_F_DEVICE_STATE  = 19
	PROTOCOL_F_MAX           = 20
)

var protocolFeatureNames = map[int]string{
	PROTOCOL_F_MQ:                   "MQ",
	PROTOCOL_F_LOG_SHMFD:            "LOG_SHMFD",
	PROTOCOL_F_RARP:                 "RARP",
	PROTOCOL_F_REPLY_ACK:            "REPLY_ACK",
	PROTOCOL_F_NET_MTU:              "NET_MTU",
	PROTOCOL_F_BACKEND_REQ:          "BACKEND_REQ",
	PROTOCOL_F_CROSS_ENDIAN:         "CROSS_ENDIAN",
	PROTOCOL_F_CRYPTO_SESSION:       "CRYPTO_SESSION",
	PROTOCOL_F_PAGEFAULT:            "PAGEFAULT",
	PROTOCOL_F_CONFIG:               "CONFIG",
	PROTOCOL_F_BACKEND_SEND_FD:      "BACKEND_SEND_FD",
	PROTOCOL_F_HOST_NOTIFIER:        "HOST_NOTIFIER",
	PROTOCOL_F_INFLIGHT_SHMFD:       "INFLIGHT_SHMFD",
	PROTOCOL_F_RESET_DEVICE:         "RESET_DEVICE",
	PROTOCOL_F_INBAND_NOTIFICATIONS: "INBAND_NOTIFICATIONS",
	PROTOCOL_F_CONFIGURE_MEM_SLOTS:  "CONFIGURE_MEM_SLOTS",
	PROTOCOL_F_STATUS:               "STATUS",
	/* Feature 17 reserved for PROTOCOL_F_XEN_MMAP. */
	PROTOCOL_F_SHARED_OBJECT: "SHARED_OBJECT",
	PROTOCOL_F_DEVICE_STATE:  "DEVICE_STATE",
	PROTOCOL_F_MAX:           "MAX",
}

// include/standard-headers/linux/virtio_config.h
// include/standard-headers/linux/vhost_types.h
const (
	F_NOTIFY_ON_EMPTY = 24
	F_LOG_ALL         = 26

	F_ANY_LAYOUT = 27

	// include/standard-headers/linux/virtio_ring.h
	//  https://stackoverflow.com/questions/46334546/what-is-indirect-buffer-and-indirect-descriptor
	RING_F_INDIRECT_DESC = 28
	RING_F_EVENT_IDX     = 29

	F_PROTOCOL_FEATURES = 30

	F_VERSION_1         = 32
	F_ACCESS_PLATFORM   = 33
	F_RING_PACKED       = 34
	F_IN_ORDER          = 35
	F_ORDER_PLATFORM    = 36
	F_SR_IOV            = 37
	F_NOTIFICATION_DATA = 38
	F_NOTIF_CONFIG_DATA = 39
	F_RING_RESET        = 40
	F_ADMIN_VQ          = 41
)

var featureNames = map[int]string{
	F_ACCESS_PLATFORM:    "ACCESS_PLATFORM",
	F_ADMIN_VQ:           "ADMIN_VQ",
	F_ANY_LAYOUT:         "ANY_LAYOUT",
	F_IN_ORDER:           "IN_ORDER",
	F_LOG_ALL:            "LOG_ALL",
	F_NOTIFICATION_DATA:  "NOTIFICATION_DATA",
	F_NOTIFY_ON_EMPTY:    "NOTIFY_ON_EMPTY",
	F_NOTIF_CONFIG_DATA:  "NOTIF_CONFIG_DATA",
	F_ORDER_PLATFORM:     "ORDER_PLATFORM",
	F_PROTOCOL_FEATURES:  "PROTOCOL_FEATURES",
	F_RING_PACKED:        "RING_PACKED",
	F_RING_RESET:         "RING_RESET",
	F_SR_IOV:             "SR_IOV",
	F_VERSION_1:          "VERSION_1",
	RING_F_EVENT_IDX:     "RING_F_EVENT_IDX",
	RING_F_INDIRECT_DESC: "RING_F_INDIRECT_DESC",
}

func maskToString(names map[int]string, mask uint64) string {
	var f []string
	for j := 0; j < 64; j++ {
		m := uint64(0x1) << j
		if mask&m != 0 {
			nm := names[j]
			if nm == "" {
				nm = strconv.Itoa(j)
			}
			f = append(f, nm)
		}
	}
	return strings.Join(f, ",")
}

// VhostUserRequest

const (
	REQ_NONE                  = 0
	REQ_GET_FEATURES          = 1
	REQ_SET_FEATURES          = 2
	REQ_SET_OWNER             = 3
	REQ_RESET_OWNER           = 4
	REQ_SET_MEM_TABLE         = 5
	REQ_SET_LOG_BASE          = 6
	REQ_SET_LOG_FD            = 7
	REQ_SET_VRING_NUM         = 8
	REQ_SET_VRING_ADDR        = 9
	REQ_SET_VRING_BASE        = 10
	REQ_GET_VRING_BASE        = 11
	REQ_SET_VRING_KICK        = 12
	REQ_SET_VRING_CALL        = 13
	REQ_SET_VRING_ERR         = 14
	REQ_GET_PROTOCOL_FEATURES = 15
	REQ_SET_PROTOCOL_FEATURES = 16
	REQ_GET_QUEUE_NUM         = 17
	REQ_SET_VRING_ENABLE      = 18
	REQ_SEND_RARP             = 19
	REQ_NET_SET_MTU           = 20
	REQ_SET_BACKEND_REQ_FD    = 21
	REQ_IOTLB_MSG             = 22
	REQ_SET_VRING_ENDIAN      = 23
	REQ_GET_CONFIG            = 24
	REQ_SET_CONFIG            = 25
	REQ_CREATE_CRYPTO_SESSION = 26
	REQ_CLOSE_CRYPTO_SESSION  = 27
	REQ_POSTCOPY_ADVISE       = 28
	REQ_POSTCOPY_LISTEN       = 29
	REQ_POSTCOPY_END          = 30
	REQ_GET_INFLIGHT_FD       = 31
	REQ_SET_INFLIGHT_FD       = 32
	REQ_GPU_SET_SOCKET        = 33
	REQ_RESET_DEVICE          = 34
	/* Message number 35 reserved for REQ_VRING_KICK. */
	REQ_GET_MAX_MEM_SLOTS   = 36
	REQ_ADD_MEM_REG         = 37
	REQ_REM_MEM_REG         = 38
	REQ_SET_STATUS          = 39
	REQ_GET_STATUS          = 40
	REQ_GET_SHARED_OBJECT   = 41
	REQ_SET_DEVICE_STATE_FD = 42
	REQ_CHECK_DEVICE_STATE  = 43
	REQ_MAX                 = 44
)

var reqNames = map[int]string{
	REQ_NONE:                  "NONE",
	REQ_GET_FEATURES:          "GET_FEATURES",
	REQ_SET_FEATURES:          "SET_FEATURES",
	REQ_SET_OWNER:             "SET_OWNER",
	REQ_RESET_OWNER:           "RESET_OWNER",
	REQ_SET_MEM_TABLE:         "SET_MEM_TABLE",
	REQ_SET_LOG_BASE:          "SET_LOG_BASE",
	REQ_SET_LOG_FD:            "SET_LOG_FD",
	REQ_SET_VRING_NUM:         "SET_VRING_NUM",
	REQ_SET_VRING_ADDR:        "SET_VRING_ADDR",
	REQ_SET_VRING_BASE:        "SET_VRING_BASE",
	REQ_GET_VRING_BASE:        "GET_VRING_BASE",
	REQ_SET_VRING_KICK:        "SET_VRING_KICK",
	REQ_SET_VRING_CALL:        "SET_VRING_CALL",
	REQ_SET_VRING_ERR:         "SET_VRING_ERR",
	REQ_GET_PROTOCOL_FEATURES: "GET_PROTOCOL_FEATURES",
	REQ_SET_PROTOCOL_FEATURES: "SET_PROTOCOL_FEATURES",
	REQ_GET_QUEUE_NUM:         "GET_QUEUE_NUM",
	REQ_SET_VRING_ENABLE:      "SET_VRING_ENABLE",
	REQ_SEND_RARP:             "SEND_RARP",
	REQ_NET_SET_MTU:           "NET_SET_MTU",
	REQ_SET_BACKEND_REQ_FD:    "SET_BACKEND_REQ_FD",
	REQ_IOTLB_MSG:             "IOTLB_MSG",
	REQ_SET_VRING_ENDIAN:      "SET_VRING_ENDIAN",
	REQ_GET_CONFIG:            "GET_CONFIG",
	REQ_SET_CONFIG:            "SET_CONFIG",
	REQ_CREATE_CRYPTO_SESSION: "CREATE_CRYPTO_SESSION",
	REQ_CLOSE_CRYPTO_SESSION:  "CLOSE_CRYPTO_SESSION",
	REQ_POSTCOPY_ADVISE:       "POSTCOPY_ADVISE",
	REQ_POSTCOPY_LISTEN:       "POSTCOPY_LISTEN",
	REQ_POSTCOPY_END:          "POSTCOPY_END",
	REQ_GET_INFLIGHT_FD:       "GET_INFLIGHT_FD",
	REQ_SET_INFLIGHT_FD:       "SET_INFLIGHT_FD",
	REQ_GPU_SET_SOCKET:        "GPU_SET_SOCKET",
	REQ_RESET_DEVICE:          "RESET_DEVICE",
	REQ_GET_MAX_MEM_SLOTS:     "GET_MAX_MEM_SLOTS",
	REQ_ADD_MEM_REG:           "ADD_MEM_REG",
	REQ_REM_MEM_REG:           "REM_MEM_REG",
	REQ_SET_STATUS:            "SET_STATUS",
	REQ_GET_STATUS:            "GET_STATUS",
	REQ_GET_SHARED_OBJECT:     "GET_SHARED_OBJECT",
	REQ_SET_DEVICE_STATE_FD:   "SET_DEVICE_STATE_FD",
	REQ_CHECK_DEVICE_STATE:    "CHECK_DEVICE_STATE",
	REQ_MAX:                   "MAX",
}

// enum VhostUserSlaveRequest

const (
	BACKEND_REQ_NONE                    = 0
	BACKEND_REQ_IOTLB_MSG               = 1
	BACKEND_REQ_CONFIG_CHANGE_MSG       = 2
	BACKEND_REQ_VRING_HOST_NOTIFIER_MSG = 3
	BACKEND_REQ_SHARED_OBJECT_ADD       = 6
	BACKEND_REQ_SHARED_OBJECT_REMOVE    = 7
	BACKEND_REQ_SHARED_OBJECT_LOOKUP    = 8
	BACKEND_REQ_MAX                     = 9
)

const (
	VHOST_MEMORY_BASELINE_NREGIONS = 8
	BACKEND_MAX_FDS                = 8
	MAX_CONFIG_SIZE                = 256
)

type GetFeaturesReply struct {
	Mask uint64
}

var decodeIn = map[uint32]func(unsafe.Pointer) interface{}{
	REQ_ADD_MEM_REG:           func(p unsafe.Pointer) interface{} { return (*VhostUserMemRegMsg)(p) },
	REQ_SET_FEATURES:          func(p unsafe.Pointer) interface{} { return (*SetFeaturesRequest)(p) },
	REQ_SET_PROTOCOL_FEATURES: func(p unsafe.Pointer) interface{} { return (*SetProtocolFeaturesRequest)(p) },
	REQ_SET_VRING_ADDR:        func(p unsafe.Pointer) interface{} { return (*VhostVringAddr)(p) },
	REQ_SET_VRING_BASE:        func(p unsafe.Pointer) interface{} { return (*VhostVringState)(p) },
	REQ_SET_VRING_CALL:        func(p unsafe.Pointer) interface{} { return (*U64Payload)(p) },
	REQ_SET_VRING_ENABLE:      func(p unsafe.Pointer) interface{} { return (*VhostVringState)(p) },
	REQ_SET_VRING_ERR:         func(p unsafe.Pointer) interface{} { return (*U64Payload)(p) },
	REQ_SET_VRING_KICK:        func(p unsafe.Pointer) interface{} { return (*U64Payload)(p) },
	REQ_SET_VRING_NUM:         func(p unsafe.Pointer) interface{} { return (*VhostVringState)(p) },
	REQ_SET_LOG_BASE:          func(p unsafe.Pointer) interface{} { return (*VhostUserLog)(p) },
}

var decodeOut = map[uint32]func(unsafe.Pointer) interface{}{
	REQ_GET_FEATURES:          func(p unsafe.Pointer) interface{} { return (*GetFeaturesReply)(p) },
	REQ_GET_PROTOCOL_FEATURES: func(p unsafe.Pointer) interface{} { return (*GetProtocolFeaturesReply)(p) },
}

var inFDCount = map[uint32]int{
	REQ_SET_BACKEND_REQ_FD: 1,
	REQ_SET_VRING_CALL:     1,
	REQ_SET_VRING_ERR:      1,
	REQ_ADD_MEM_REG:        1,
	REQ_SET_VRING_KICK:     1,
	REQ_SET_LOG_BASE:       1,
}

func (r *GetFeaturesReply) String() string {
	return fmt.Sprintf("{%s}",
		maskToString(featureNames, r.Mask))
}

type SetFeaturesRequest struct {
	Mask uint64
}

func (r *SetFeaturesRequest) String() string {
	return fmt.Sprintf("{%s}",
		maskToString(featureNames, r.Mask))
}

type GetProtocolFeaturesReply struct {
	Mask uint64
}

func (r *GetProtocolFeaturesReply) String() string {
	return fmt.Sprintf("{%s}",
		maskToString(protocolFeatureNames, r.Mask))
}

type SetProtocolFeaturesRequest struct {
	Mask uint64
}

func (r *SetProtocolFeaturesRequest) String() string {
	return fmt.Sprintf("{%s}",
		maskToString(protocolFeatureNames, r.Mask))
}

type U64Payload struct {
	Num uint64
}

func (p *U64Payload) String() string {
	return fmt.Sprintf("{%d}", p.Num)
}

type VhostVringState struct {
	Index uint32
	Num   uint32 // unsigned int?
}

func (s *VhostVringState) String() string {
	return fmt.Sprintf("idx %d num %d", s.Index, s.Num)
}

type VhostVringAddr struct {
	Index uint32
	/* Option flags. */
	Flags uint32
	/* Flag values: */
	/* Whether log address is valid. If set enables logging. */
	//#define VHOST_VRING_F_LOG 0

	/* Start of array of descriptors (virtually contiguous) */
	DescUserAddr uint64
	/* Used structure address. Must be 32 bit aligned */
	UsedUserAddr uint64
	/* Available structure address. Must be 16 bit aligned */
	AvailUserAddr uint64
	/* Logging support. */
	/* Log writes to used structure, at offset calculated from specified
	 * address. Address must be 32 bit aligned. */
	LogGuestAddr uint64
}

func (a *VhostVringAddr) String() string {
	return fmt.Sprintf("idx %d flags %x Desc %x Used %x Avail %x LogGuest %x",
		a.Index, a.Flags, a.DescUserAddr, a.UsedUserAddr,
		a.AvailUserAddr, a.LogGuestAddr)
}

// virtio_ring.h

// must be aligned on 4 bytes, but that's automatic?
type VringUsedElement struct {
	ID  uint32
	Len uint32
}

func (ue *VringUsedElement) String() string {
	return fmt.Sprintf("{id: %d len: %d}", ue.ID, ue.Len)
}

// aligned 4 bytes
type VringUsed struct {
	Flags uint16
	Idx   uint16
	Ring0 VringUsedElement
}

// qemu:include/standard-headers/linux/virtio_ring.h
const (
	/* This marks a buffer as continuing via the next field. */
	VRING_DESC_F_NEXT = 1
	/* This marks a buffer as write-only (otherwise read-only). */
	VRING_DESC_F_WRITE = 2
	/* This means the buffer contains a list of buffer descriptors. */
	VRING_DESC_F_INDIRECT = 4
)

var vringDescNames = map[int]string{
	0: "NEXT",
	1: "WRITE",
	2: "INDIRECT",
}

// Aligned 16 byte

type VringDesc struct {
	Addr  uint64
	Len   uint32
	Flags uint16
	Next  uint16
}

func (d VringDesc) String() string {
	return fmt.Sprintf("[0x%x,+0x%x) %s next %d", d.Addr, d.Len, maskToString(vringDescNames, uint64(d.Flags)), d.Next)
}

// aligned on 2 bytes

type VringAvail struct {
	Flags uint16
	Idx   uint16
	Ring0 uint16
}

type VhostUserMemoryRegion struct {
	GuestPhysAddr uint64
	MemorySize    uint64
	DriverAddr    uint64
	MmapOffset    uint64
}

func (r *VhostUserMemoryRegion) String() string {
	return fmt.Sprintf("Guest [0x%x,+0x%x) Driver %x MmapOff %x",
		r.GuestPhysAddr, r.MemorySize, r.DriverAddr, r.MmapOffset)
}

type VhostUserMemory struct {
	Nregions uint32
	Padding  uint32
	Regions  [VHOST_MEMORY_BASELINE_NREGIONS]VhostUserMemoryRegion
}

type VhostUserMemRegMsg struct {
	Padding uint64
	Region  VhostUserMemoryRegion
}

type VhostUserLog struct {
	MmapSize   uint64
	MmapOffset uint64
}

func (l *VhostUserLog) String() string {
	return fmt.Sprintf("[0x%x,+0x%x)", l.MmapSize, l.MmapOffset)
}

type VhostUserConfig struct {
	Offset uint32
	Size   uint32
	Flags  uint32
	Region [MAX_CONFIG_SIZE]uint8
}

type VhostUserVringArea struct {
	U64    uint64
	Size   uint64
	Offset uint64
}

type VhostUserInflight struct {
	MmapSize   uint64
	MmapOffset uint64
	NumQueues  uint16
	QueueSize  uint16
}

type VhostUserShared struct {
	Uuid [16]byte
}

type Header struct {
	Request uint32
	/*
			VERSION_MASK     (0x3)
		        USER_REPLY  (0x1 << 2)
		        NEED_REPLY  (0x1 << 3)
	*/
	Flags uint32
	/* the following payload size */
	Size uint32
}

/* Request payload of VHOST_USER_SET_DEVICE_STATE_FD */
type VhostUserTransferDeviceState struct {
	Direction uint32
	Phase     uint32
}

/* no alignment requirement */
type VhostIotlbMsg struct {
	Iova  uint64
	Size  uint64
	Uaddr uint64
	/*
		#define VHOST_ACCESS_RO      0x1
		#define VHOST_ACCESS_WO      0x2
		#define VHOST_ACCESS_RW      0x3
	*/
	Perm uint8
	/*
		#define VHOST_IOTLB_MISS           1
		#define VHOST_IOTLB_UPDATE         2
		#define VHOST_IOTLB_INVALIDATE     3
		#define VHOST_IOTLB_ACCESS_FAIL    4
	*/
	/*
	 * VHOST_IOTLB_BATCH_BEGIN and VHOST_IOTLB_BATCH_END allow modifying
	 * multiple mappings in one go: beginning with
	 * VHOST_IOTLB_BATCH_BEGIN, followed by any number of
	 * VHOST_IOTLB_UPDATE messages, and ending with VHOST_IOTLB_BATCH_END.
	 * When one of these two values is used as the message type, the rest
	 * of the fields in the message are ignored. There's no guarantee that
	 * these changes take place automatically in the device.
	 */
	/*
		#define VHOST_IOTLB_BATCH_BEGIN    5
		#define VHOST_IOTLB_BATCH_END      6
	*/
	Type uint8
}
