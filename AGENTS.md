# Notes for coding agents

Go bindings for the FUSE kernel protocol. Module path:
`github.com/hanwen/go-fuse/v3`. Minimum Go version is in `go.mod`; don't
use language or library features newer than that.

## Layout

* `fuse/` — raw protocol: wire types, request parsing, the server loop,
  mounting. `RawFileSystem` is the low-level API.
* `fs/` — the recommended high-level, inode-based API built on `fuse/`.
  Design rationale is in `fs/README.md`.
* `posixtest/` — POSIX conformance tests, reused by `fs`, `virtiofs` and
  the FreeBSD run. Add filesystem-semantics tests here when they apply to
  any backend.
* `virtiofs/`, `internal/vhostuser/` — serving FUSE to VMs over
  vhost-user.
* `splice/`, `internal/*` — platform helpers.
* `fskit/` — serving FUSE through macOS FSKit.
* `winfsp/` — serving FUSE on Windows through WinFsp. The callback
  translation is tested on Linux; the mount tests run on Windows.
* `internal/fuseclient` — an in-process FUSE "kernel" that sends
  requests to a `fuse.ProtocolServer`; used by `winfsp`, and usable
  to drive file systems in tests without a mount.
* `zipfs/`, `newunionfs/`, `example/` — example filesystems.
* `benchmark/` — benchmarks; `compare.bash` compares two revisions.
* `kernel/` — run the tests against a custom Linux kernel in QEMU.
* `docs/` — release notes (`v2.md`, `v3.md`), the v2 to v3 upgrade
  skill (`skills/upgrade-go-fuse-v3/`), `macos.md`, `windows.md` and
  `comparison.md`.

Platform-specific code uses `_linux.go`, `_darwin.go`, `_freebsd.go`,
`_openbsd.go`, `_unix.go` suffixes; `_bsd.go` files are shared by
FreeBSD and OpenBSD. Keep the shared file free of OS-specific syscalls.

## Building and testing

```
go build ./...
GOOS=darwin go build ./fs/... ./example/loopback/...
GOOS=freebsd go build ./fs/... ./example/loopback/...
GOOS=openbsd go build ./fs/... ./example/loopback/...
GOOS=windows go build ./fs/... ./winfsp/...
go test -timeout 5m -p 1 -count 1 ./...
```

The cross-builds must keep passing. `all.bash` is what CI runs. Tests
mount real FUSE filesystems, so they need `/dev/fuse` and `fusermount3`.
Some tests need root (see the `sudo` lines in `all.bash`) and skip
otherwise. CI also runs with `GOMAXPROCS=1`; some races only show up
there. `virtiofs` tests need KVM and QEMU or cloud-hypervisor (pick with
`GO_FUSE_VMM=qemu|cloud-hypervisor`), and skip silently without them.
`test-freebsd.bash` runs `posixtest` and `fs` in a FreeBSD VM;
`openbsd/test.bash` runs the `fs` TestPosix in an OpenBSD VM.

Almost all tests should be mounting an actual file system against
/dev/fuse, and interact through the mountpoint.

If `fusermount3` can't mount (e.g. in a sandbox where it isn't
setuid-root), run the tests in a user+mount namespace and hide the
helper, so go-fuse falls back to calling `mount(2)` directly:

```
unshare -rm sh -c 'mount --bind /dev/null /usr/bin/fusermount3 &&
  go test -count 1 -run TestXxx ./fs'
```

Inside the namespace the euid is 0, so tests that check for root run
instead of skipping. Expect failures from tests that need real
privileges or other uids (eg. `TestForget`, `TestDefaultOwner`,
`TestPassthrough`, `TestStatx`, `TestReconnectByFileHandle`); compare
against a run without your change.

Use `-v` to get FUSE protocol debug logging (`testutil.VerboseTest`).

Run `gofmt` and `go vet` on changed packages.

## Style

* Keep comments minimal; document exported API, not the obvious.
* New files start with the BSD license header:
  `// Copyright <year> the Go-FUSE Authors. All rights reserved.`
  (copy the three lines from an existing file).
* Match the kernel's names for protocol structs and fields
  (`include/uapi/linux/fuse.h`).

## API changes

Record user-visible API and behavior changes in `docs/v3.md`. If a
change breaks v2 code, also add the fix to
`docs/skills/upgrade-go-fuse-v3/SKILL.md`.

## Commits

The repo is managed with jj (`.jj/` is colocated); prefer `jj` over raw
git for history edits. Review happens on Gerrit (see `CONTRIBUTING`),
so each commit is one reviewable change.

Commit message format:

```
pkg: short imperative summary

Body explaining why.

Change-Id: I<40 hex chars>
```

`pkg` is the package touched (`fuse`, `fs`, `fuse, fs`, `posixtest`,
...). Keep the `Change-Id` when amending an existing change.
