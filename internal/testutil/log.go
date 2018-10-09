// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import "log"

func init() {
	// For test, the date is irrelevant, but microseconds are.
	log.SetFlags(log.Lmicroseconds)
}
