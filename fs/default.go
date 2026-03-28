// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"time"
)

// DefaultOptions returns the default Options that are used when
// nil is passed for *Options.
//
// When you do want to set something in Options, get the defaults
// from this function and adjust as required.
func DefaultOptions() *Options {
	oneSec := time.Second
	return &Options{
		// libfuse also uses one second per default
		EntryTimeout:      &oneSec,
		AttrTimeout:       &oneSec,
		FirstAutomaticIno: 1 << 63,
	}
}
