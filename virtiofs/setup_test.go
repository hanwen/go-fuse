// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"encoding/binary"
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
const busyboxURL = "https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"

// testBinDir is where downloaded/detected assets are cached across runs.
var testBinDir = filepath.Join(os.Getenv("HOME"), ".cache", "go-fuse-virtiofs")

const (
	vmmQemu            = "qemu"
	vmmCloudHypervisor = "cloud-hypervisor"
)

// testAssets holds the paths resolved by TestMain.
var testAssets struct {
	// vmm is vmmQemu or vmmCloudHypervisor.
	vmm     string
	vmmBin  string
	busybox string
	// kernel is a bzImage for QEMU, and an ELF vmlinux for cloud-hypervisor.
	kernel string
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

// findVMM picks the VMM from $GO_FUSE_VMM ("qemu" or
// "cloud-hypervisor"), or else the first one found in $PATH.
func findVMM() error {
	// TODO: arm64 needs an aarch64 busybox and a non-"pc" machine type.
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("not supported: %s", runtime.GOARCH)
	}
	bins := map[string]string{
		vmmQemu:            "qemu-system-x86_64",
		vmmCloudHypervisor: "cloud-hypervisor",
	}
	candidates := []string{vmmQemu, vmmCloudHypervisor}
	if v := os.Getenv("GO_FUSE_VMM"); v != "" {
		if _, ok := bins[v]; !ok {
			return fmt.Errorf("GO_FUSE_VMM=%q: want %q or %q", v, vmmQemu, vmmCloudHypervisor)
		}
		candidates = []string{v}
	}

	var errs []string
	for _, vmm := range candidates {
		bin, err := exec.LookPath(bins[vmm])
		if err == nil {
			testAssets.vmm = vmm
			testAssets.vmmBin = bin
			return nil
		}
		errs = append(errs, err.Error())
	}
	return fmt.Errorf("no VMM found: %s", strings.Join(errs, "; "))
}

func prepareAssets() error {
	if err := findVMM(); err != nil {
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
	if testAssets.vmm == vmmCloudHypervisor {
		// cloud-hypervisor only boots ELF kernels with a PVH entry point.
		kernel, err = ensureVmlinux(kernel)
		if err != nil {
			return fmt.Errorf("vmlinux: %w", err)
		}
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

// ensureVmlinux extracts the ELF kernel from the bzImage at path, caching
// the result in testBinDir.
func ensureVmlinux(bzImage string) (string, error) {
	p := filepath.Join(testBinDir, "vmlinux-"+strings.TrimPrefix(filepath.Base(bzImage), "vmlinuz-"))
	if fi, err := os.Stat(p); err == nil {
		if bzi, err := os.Stat(bzImage); err == nil && !fi.ModTime().Before(bzi.ModTime()) {
			return p, nil
		}
	}

	img, err := os.ReadFile(bzImage)
	if err != nil {
		return "", err
	}
	vmlinux, err := extractVmlinux(img)
	if err != nil {
		return "", fmt.Errorf("%s: %w", bzImage, err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, vmlinux, 0644); err != nil {
		return "", err
	}
	return p, os.Rename(tmp, p)
}

// extractVmlinux decompresses the payload of an x86 bzImage, see
// Documentation/arch/x86/boot.rst.
func extractVmlinux(img []byte) ([]byte, error) {
	if len(img) < 0x250 || string(img[0x202:0x206]) != "HdrS" {
		return nil, fmt.Errorf("not a bzImage")
	}
	if version := binary.LittleEndian.Uint16(img[0x206:]); version < 0x208 {
		return nil, fmt.Errorf("boot protocol %x too old", version)
	}
	setupSects := int(img[0x1f1])
	if setupSects == 0 {
		setupSects = 4
	}
	start := (setupSects+1)*512 + int(binary.LittleEndian.Uint32(img[0x248:]))
	end := start + int(binary.LittleEndian.Uint32(img[0x24c:]))
	if start >= end || end > len(img) {
		return nil, fmt.Errorf("payload [%d,%d) out of bounds", start, end)
	}
	payload := img[start:end]

	var out []byte
	var err error
	if bytes.HasPrefix(payload, []byte{0x1f, 0x8b}) {
		var r *gzip.Reader
		if r, err = gzip.NewReader(bytes.NewReader(payload)); err == nil {
			out, err = io.ReadAll(r)
		}
	} else {
		// Except for gzip, the kernel appends the uncompressed size.
		if len(payload) < 4 {
			return nil, fmt.Errorf("payload too short")
		}
		size := binary.LittleEndian.Uint32(payload[len(payload)-4:])
		payload = payload[:len(payload)-4]

		var tool []string
		switch {
		case bytes.HasPrefix(payload, []byte("BZh")):
			out, err = io.ReadAll(bzip2.NewReader(bytes.NewReader(payload)))
		case bytes.HasPrefix(payload, []byte{0x28, 0xb5, 0x2f, 0xfd}):
			tool = []string{"zstd", "-dc"}
		case bytes.HasPrefix(payload, []byte{0xfd, '7', 'z', 'X', 'Z', 0}):
			tool = []string{"xz", "-dc"}
		case bytes.HasPrefix(payload, []byte{0x5d, 0, 0}):
			tool = []string{"xz", "--format=lzma", "-dc"}
		case bytes.HasPrefix(payload, []byte{0x89, 'L', 'Z', 'O'}):
			tool = []string{"lzop", "-dc"}
		case bytes.HasPrefix(payload, []byte{0x02, 0x21, 0x4c, 0x18}):
			tool = []string{"lz4", "-dc"}
		default:
			return nil, fmt.Errorf("unknown compression % x", payload[:min(len(payload), 6)])
		}
		if tool != nil {
			cmd := exec.Command(tool[0], tool[1:]...)
			cmd.Stdin = bytes.NewReader(payload)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if out, err = cmd.Output(); err != nil {
				err = fmt.Errorf("%v: %w: %s", tool, err, stderr.Bytes())
			}
		}
		if err == nil && uint32(len(out)) != size {
			err = fmt.Errorf("decompressed %d bytes, want %d", len(out), size)
		}
	}
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(out, []byte("\x7fELF")) {
		return nil, fmt.Errorf("payload is not ELF")
	}
	return out, nil
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
		if after, ok := strings.CutPrefix(line, "insmod "); ok {
			paths = append(paths, after)
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
