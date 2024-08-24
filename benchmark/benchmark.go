// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

// Routines for benchmarking fuse.

import (
	"bytes"
	"log"
	"os"
)

func ReadLines(name string) []string {
	data, err := os.ReadFile(name)
	if err != nil {
		log.Fatal("ReadFile: ", err)
	}

	var lines []string
	for _, l := range bytes.Split(data, []byte("\n")) {
		if len(l) > 0 {
			lines = append(lines, string(l))
		}
	}
	return lines
}
