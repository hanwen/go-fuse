# Notes for coding agents

Go bindings for the FUSE kernel protocol. Module path:
`github.com/hanwen/go-fuse/v2`. Minimum Go version is in `go.mod`; don't
use language or library features newer than that.

## Layout

* `fuse/` — raw protocol: wire types, request parsing, the server loop,
  mounting. `RawFileSystem` is the low-level API.
* `fs/` — the recommended high-level, inode-based API built on `fuse/`.
  Design rationale is in `fs/README.md`.
* `fuse/nodefs`, `fuse/pathfs` — deprecated; only fix bugs there.
* `posixtest/` — POSIX conformance tests, reused by `fs`, `virtiofs` and
  the FreeBSD run. Add filesystem-semantics tests here when they apply to
  any backend.
* `virtiofs/`, `internal/vhostuser/` — serving FUSE to VMs over
  vhost-user.
* `splice/`, `internal/*` — platform helpers.
* `zipfs/`, `newunionfs/`, `example/` — example filesystems.

Platform-specific code uses `_linux.go`, `_darwin.go`, `_freebsd.go`,
`_unix.go` suffixes. Keep the shared file free of OS-specific syscalls.

## Building and testing

```
go build ./...
GOOS=darwin go build ./fs/... ./example/loopback/...
GOOS=freebsd go build ./fs/... ./example/loopback/...
go test -timeout 5m -p 1 -count 1 ./...
```

Both cross-builds must keep passing. `all.bash` is what CI runs. Tests
mount real FUSE filesystems, so they need `/dev/fuse` and `fusermount3`.
Some tests need root (see the `sudo` lines in `all.bash`) and skip
otherwise. CI also runs with `GOMAXPROCS=1`; some races only show up
there. `virtiofs` tests need QEMU/KVM and skip silently without them.
`test-freebsd.bash` runs `posixtest` and `fs` in a FreeBSD VM.

Almost all tests should be mounting an actual file system against
/dev/fuse, and interact through the mountpoint.

Use `-v` to get FUSE protocol debug logging (`testutil.VerboseTest`).

Run `gofmt` and `go vet` on changed packages.

## Style

* Keep comments minimal; document exported API, not the obvious.
* New files start with the BSD license header:
  `// Copyright <year> the Go-FUSE Authors. All rights reserved.`
  (copy the three lines from an existing file).
* Match the kernel's names for protocol structs and fields
  (`include/uapi/linux/fuse.h`).

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
