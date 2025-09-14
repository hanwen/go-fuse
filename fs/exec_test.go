// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"os/exec"
	"strings"
	"testing"
)

func TestExec(t *testing.T) {
	tc := newTestCase(t, &testOptions{
		attrCache:  true,
		entryCache: true,
	})
	defer tc.clean()

	scriptContent := "#!/bin/sh\necho hello"
	tc.writeOrig("test.sh", scriptContent, 0755)

	cmd := exec.Command(tc.mntDir + "/test.sh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exec failed: %v, output: %s", err, out)
	}

	if got := strings.TrimSpace(string(out)); got != "hello" {
		t.Errorf("got output %q, want %q", got, "hello")
	}
}
