# macOS support

go-fuse runs on macOS in two ways: through the macFUSE kernel
extension, or through FSKit. Both need [macFUSE](https://macfuse.github.io/)
(see its [Getting Started](https://github.com/macfuse/macfuse/wiki/Getting-Started)
and [FUSE Backends](https://github.com/macfuse/macfuse/wiki/FUSE-Backends)
guides). FUSE-T is not supported. The macFUSE and FSKit details below
are accurate as of November 2026.

The main developer (hanwen@) does not own a Mac, but accepts patches
to make go-fuse work better on macOS. Tests are expected to pass;
report any failure as a bug.

## Common limitations (Kernel extension and FSKit)

These apply to both the macFUSE kernel extension and FSKit:

* macFUSE speaks
  [protocol version 7.19](https://github.com/macfuse/macfuse/wiki/FUSE-Features),
  so newer FUSE features are not available (see
  [Protocol version 7.19](#protocol-version-719) below).
* There is no splice and no passthrough.
* No POSIX record locking (`GETLK`, `SETLK`, `SETLKW`), which macFUSE
  lists as
  [unsupported](https://github.com/macfuse/macfuse/wiki/FUSE-Features).
* macOS issues STATFS calls continuously. This costs performance.
* The file system must answer STATFS, or the mount fails. Both `fuse`
  (when `RawFileSystem.StatFs` returns `ENOSYS` on Darwin) and `fs`
  (when a node does not implement `NodeStatfser`) return a zeroed
  result automatically.
* macFUSE times out operations after 60 seconds by default. go-fuse
  turns this off with
  [`daemon_timeout=0`](https://github.com/macfuse/macfuse/wiki/Mount-Options),
  to match Linux.
* `LoopbackNode.Mknod` is path based, because Darwin has no public
  `mknodat`.
* Kernel notifications are limited or unavailable (see the backend
  sections below). Missing notifications only matter if the file system
  uses long cache timeouts (`EntryTimeout`, `AttrTimeout`,
  `NegativeTimeout`) or `FOPEN_KEEP_CACHE` and expects to push
  invalidations when backing data changes out of band; with zero or
  short timeouts, changes are picked up on the next lookup or open
  without notifications.

## macFUSE kernel extension

This is the default. `fuse.NewServer` and `fs.Mount` work as on Linux.
They run `mount_macfuse` (or the older `mount_osxfuse`) to mount the
file system. On Apple Silicon Macs, loading the kernel extension
requires enabling third-party kernel extensions in Recovery Mode (see
[Getting Started](https://github.com/macfuse/macfuse/wiki/Getting-Started)).

Additional limitations:

* Requests are read by one goroutine. Concurrent reads from the FUSE
  device race on unmount, which can make unmount hang.
* `Server.Stop` cannot interrupt a reader that waits for a request.
  The reader stops after the next request arrives.
* Kernel notifications (`EntryNotify`, `InodeNotify`, etc.) are only
  tested on Linux. macFUSE does not support `Store` or `Retrieve`
  ([FUSE Features](https://github.com/macfuse/macfuse/wiki/FUSE-Features)),
  `Prune` (protocol 7.45), or `CAP_EXPLICIT_INVAL_DATA`.

## FSKit

The `fskit` package mounts through macFUSE's
[FSKit backend](https://github.com/macfuse/macfuse/wiki/FUSE-Backends),
using the user-space framework in macFUSE 5.3.0 or newer on macOS
15.4 or newer. It does not need the kernel extension. It loads the
framework at run time with
[purego](https://github.com/ebitengine/purego), so it does not need
cgo.

Use `fskit.Mount` for an `fs` tree, or `fskit.MountRaw` for a
`fuse.RawFileSystem`. Use `fskit.Available` to check that the
framework is installed.

Setup (see macFUSE's
[Getting Started](https://github.com/macfuse/macfuse/wiki/Getting-Started)):

* Open the macFUSE app once, to install the helper tools and register
  the FSKit extension.
* Enable the extension in System Settings → General → Login Items &
  Extensions → File System Extensions.

The error from `Mount` says which step is missing.

Additional limitations:

* Kernel notifications are not supported at all. `EntryNotify`,
  `InodeNotify` and the other notify calls return ENOSYS.
* Most mount options handled by the kernel extension are not yet
  implemented by macFUSE's FSKit backend, and performance is not yet
  on par with the kernel extension
  ([FUSE Backends](https://github.com/macfuse/macfuse/wiki/FUSE-Backends)).
* `SETXATTR` failures on `com.apple.*` attributes are masked as
  success by `fskit`, because FSKit treats them as fatal to file
  creation.
* The mount is served by `fuse.ProtocolServer`, not `fuse.Server`.
  So `Server.Stop`, `Server.Dev` and handing the mount to another
  process are not available.

## Protocol version 7.19

The FUSE kernel protocol is versioned as `major.minor`, negotiated in
`FUSE_INIT`. macFUSE stops at
[7.19](https://github.com/macfuse/macfuse/wiki/FUSE-Features) (from
Linux 3.5, 2012), compared to 7.47 on Linux in go-fuse.

macFUSE adds some Darwin-specific extensions (such as `crtime`,
`CAP_RENAME_SWAP`, `CAP_RENAME_EXCL` and `CAP_CASE_INSENSITIVE`), but
features added to Linux FUSE after 7.19 are not available on macOS.
Notable examples include:

* Caching and performance: `READDIRPLUS` (7.21), writeback cache
  (7.23), stateless open and directory/symlink caching (`FOPEN_CACHE_DIR`,
  `CAP_CACHE_SYMLINKS`, 7.23–7.28), parallel directory operations
  (7.25), explicit data invalidation (`CAP_EXPLICIT_INVAL_DATA`), and
  passthrough (7.40).
* Operations and protocol features: `LSEEK` (7.24),
  `COPY_FILE_RANGE` (7.28), 64-bit capability flags (`CAP_INIT_EXT`,
  7.36), `STATX` (7.39), `NOTIFY_PRUNE` (7.45), `TMPFILE`, POSIX
  ACLs, and ID-mapped mounts.
