# Windows support

The `winfsp` package mounts go-fuse file systems on Windows, through
[WinFsp](https://winfsp.dev/). File systems written against the `fs`
package or `fuse.RawFileSystem` work unchanged, as long as they build
on Windows; `fs.LoopbackNode`, for example, does not. The package does
not need cgo: it loads the WinFsp DLL at run time.

How the mount behaves differently from Linux (names, ownership,
read-only files, deletion, symlinks, caching, notifications) is
described in the package documentation, in `winfsp/doc.go`. So is
the licensing of WinFsp.

## Setup

Install WinFsp from <https://winfsp.dev/rel/>; the "Core" component is
enough. The package finds the DLL through the registry key that the
installer writes. Mounting needs no administrator rights.

Only amd64 and arm64 are supported. WinFsp also has a 32-bit (386)
build, but Go's Windows callbacks cannot take its 64-bit arguments,
and its calling convention differs; the comment above `initInterface`
in `winfsp_windows.go` explains what 386 support would need.

`winfsp.Available` reports whether WinFsp is installed.

## Usage

```go
root := &fs.MemDir{Attr: fuse.Attr{Mode: 0755}}
server, err := winfsp.Mount("X:", root, nil)
if err != nil {
	log.Fatal(err)
}
defer server.Unmount()
server.Wait()
```

The mount point is a drive letter (`"X:"`), a directory that does not
exist yet, or `""`, for which WinFsp picks a free drive letter.
`winfsp.Mount` mounts an `fs` tree, like `fs.Mount`;
`winfsp.MountRaw` mounts a `fuse.RawFileSystem`. Its documentation
lists which `fuse.MountOptions` apply.

## Design

WinFsp is a kernel driver plus a DLL. The DLL calls a table of
path-based callbacks (`Create`, `Open`, `Read`, `ReadDirectory`,
`Cleanup`, ...) from its own threads. The package registers these
with `windows.NewCallback`, and translates each call into FUSE
requests, which it passes to a `fuse.ProtocolServer`. So debug
logging, `PanicHandler` and protocol negotiation work as on Linux.

The translation acts as a small FUSE kernel (`internal/fuseclient`):
it resolves paths to node IDs with LOOKUP, caches entries for their
entry timeout, keeps lookup counts, and sends FORGET when a node is
no longer used. On unmount it releases open files, forgets all nodes
and sends DESTROY. Directory listings are read in full on the first
`ReadDirectory`, because WinFsp resumes listings by name rather than
by offset.

WinFsp also offers a libfuse-compatible API, which cgofuse uses. That
API is path based, so it does not fit the go-fuse inode model. Dokany
and ProjFS were not used: Dokany is slower and less complete, and
ProjFS is meant for placeholder files rather than general read-write
file systems.

## Testing

The translation is tested on Linux, in `winfsp/translate_test.go`, by
calling the WinFsp callbacks directly against an in-memory file
system. These tests are part of a normal `go test ./...`.

`winfsp/winfsp_windows_test.go` mounts through WinFsp and uses the
mount with the `os` package. These tests skip if WinFsp is not
installed.
