// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"runtime"
	"strings"
)

const (
	// defaultMaxWrite is the default value for MountOptions.MaxWrite
	defaultMaxWrite = 128 * 1024 // 128 kiB

	_MAX_NAME_LEN = 20
)

func (o *MountOptions) setDefaults(fs RawFileSystem) {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	if o.MaxWrite < 0 {
		o.MaxWrite = 0
	}
	if o.MaxWrite == 0 {
		o.MaxWrite = defaultMaxWrite
	}
	kernelMaxWrite := getMaxWrite()
	if o.MaxWrite > kernelMaxWrite {
		o.MaxWrite = kernelMaxWrite
	}
	if o.MaxInflightRequestBytes <= 0 {
		o.MaxInflightRequestBytes = math.MaxInt
	}
	if o.MaxStackDepth == 0 {
		o.MaxStackDepth = 1
	}
	if o.Name == "" && fs != nil {
		var name string
		if s, ok := fs.(fmt.Stringer); ok {
			name = s.String()
		} else {
			t := reflect.TypeOf(fs)
			for t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			name = t.Name()
		}
		l := min(len(name), _MAX_NAME_LEN)
		o.Name = strings.Replace(name[:l], ",", ";", -1)
	}
	if o.PanicHandler == nil {
		l := o.Logger
		o.PanicHandler = func(obj any) Status {
			return defaultPanicHandler(l, obj)
		}
	}

	for _, s := range []struct {
		flag bool
		mask uint64
	}{
		{o.SyncRead, CAP_ASYNC_READ},
		{o.DisableReadDirPlus, CAP_READDIRPLUS},
		{!o.IDMappedMount, CAP_ALLOW_IDMAP},
	} {
		if s.flag {
			o.DisabledCapabilities |= s.mask
		}
	}
}

func defaultPanicHandler(logger *log.Logger, obj any) Status {
	const size = 64 << 10
	buf := make([]byte, size)
	buf = buf[:runtime.Stack(buf, false)]
	logger.Printf("panic in FS handler: %v\n%s", obj, buf)
	return EIO
}

// SupportsVersion returns true if the kernel supports the given
// protocol version or newer.
func (in *InitIn) SupportsVersion(maj, min uint32) bool {
	return in.Major > maj || (in.Major == maj && in.Minor >= min)
}

func (in *InitIn) supportsNotify(c notifyCode) bool {
	switch c {
	case notifyInvalEntry, notifyInvalInode:
		return in.SupportsVersion(7, 12)
	case notifyStore, notifyRetrieve:
		return in.SupportsVersion(7, 15)
	case notifyDelete:
		return in.SupportsVersion(7, 18)
	case notifyPrune:
		return in.SupportsVersion(7, 45)
	}
	return false
}
