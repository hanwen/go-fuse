// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"os"
	"strings"
)

// InUserNamespace reports whether the process runs in a user
// namespace other than the initial one, such as one created by
// "unshare -rm". There, only the mapped uids and gids can be used.
func InUserNamespace() bool {
	data, err := os.ReadFile("/proc/self/uid_map")
	if err != nil {
		return false
	}
	return strings.Join(strings.Fields(string(data)), " ") != "0 0 4294967295"
}

// RealRoot reports whether the process runs as root in the initial
// user namespace.
func RealRoot() bool {
	return os.Geteuid() == 0 && !InUserNamespace()
}
