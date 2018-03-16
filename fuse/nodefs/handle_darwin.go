// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

func init() {
	// darwin / osxfuse has problems with handle reuse, we get
	// the kernel message
	//    osxfuse: vnode changed generation
	// and errors returned to the user. simpleHandleMap never reuses handles
	// which seems to keep osxfuse happy.
	// https://github.com/hanwen/go-fuse/issues/204
	newHandleMap = newSimpleHandleMap
}
