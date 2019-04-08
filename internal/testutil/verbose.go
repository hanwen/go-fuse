// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"os"
)

// VerboseTest returns true if the testing framework is run DEBUG=1.
func VerboseTest() bool {
	val := os.Getenv("DEBUG")
	return val == "1"
}
