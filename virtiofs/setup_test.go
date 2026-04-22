// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// busyboxURL is the official static x86-64 busybox binary from busybox.net.
// TODO: support arm64 as well.
const busyboxURL = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"

// testBinDir is where downloaded/detected assets are cached across runs.
var testBinDir = filepath.Join(os.Getenv("HOME"), ".cache", "go-fuse-virtiofs")

// testAssets holds the paths resolved by TestMain.
var testAssets struct {
	qemuBin string
	busybox string
	kernel  string
	// modules is the ordered list of host .ko paths to embed in the initrd.
	// Empty when virtiofs is compiled into the host kernel.
	modules []string
}

func TestMain(m *testing.M) {
	if err := prepareAssets(); err != nil {
		fmt.Fprintf(os.Stderr, "virtiofs tests skipped: %v\n", err)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func prepareAssets() error {
	byArch := map[string]string{
		"amd64": "qemu-system-x86_64",
		"arm64": "qemu-system-aarch64",
	}
	bin, ok := byArch[runtime.GOARCH]
	if !ok {
		return fmt.Errorf("not supported: %s", runtime.GOARCH)
	}

	var err error
	testAssets.qemuBin, err = exec.LookPath(bin)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(testBinDir, 0755); err != nil {
		return err
	}

	busybox, err := ensureBusybox()
	if err != nil {
		return fmt.Errorf("busybox: %w", err)
	}
	testAssets.busybox = busybox

	kernel, err := findHostKernel()
	if err != nil {
		return fmt.Errorf("kernel: %w", err)
	}
	testAssets.kernel = kernel

	mods, err := findVirtioFSModules()
	if err != nil {
		return fmt.Errorf("virtiofs module: %w", err)
	}
	testAssets.modules = mods

	return nil
}

// ensureBusybox returns a path to a static busybox binary, downloading it if
// it is not already cached in testBinDir.
func ensureBusybox() (string, error) {
	p := filepath.Join(testBinDir, "busybox")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}

	fmt.Fprintf(os.Stderr, "downloading %s\n", busyboxURL)
	resp, err := http.Get(busyboxURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: HTTP %d", busyboxURL, resp.StatusCode)
	}

	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return p, os.Rename(tmp, p)
}

// findHostKernel returns the path to the running kernel's bzImage.
func findHostKernel() (string, error) {
	rel, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "", err
	}
	release := strings.TrimSpace(string(rel))
	candidates := []string{
		"/boot/vmlinuz-" + release,
		"/boot/vmlinuz",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("kernel image not found (tried %v)", candidates)
}

// findVirtioFSModules returns the ordered list of host module paths needed to
// load virtiofs.  Returns nil (no error) when virtiofs is compiled into the
// kernel (builtin), in which case no insmod calls are needed.
func findVirtioFSModules() ([]string, error) {
	out, err := exec.Command("modprobe", "--show-depends", "virtiofs").Output()
	if err != nil {
		return nil, fmt.Errorf("modprobe --show-depends virtiofs: %w (is virtiofs available?)", err)
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "builtin") {
			return nil, nil // compiled in, nothing to load
		}
		if strings.HasPrefix(line, "insmod ") {
			paths = append(paths, strings.TrimPrefix(line, "insmod "))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no module paths found in modprobe output")
	}
	return paths, nil
}

// moduleInsmodLines returns the init-script fragment that loads virtiofs
// modules.  The modules are placed at /lib/<basename> (without compression
// suffix) inside the initrd by mkinitRam.
func moduleInsmodLines(modules []string) string {
	var sb strings.Builder
	for _, p := range modules {
		base := stripCompression(filepath.Base(p))
		fmt.Fprintf(&sb, "insmod /lib/%s\n", base)
	}
	return sb.String()
}
