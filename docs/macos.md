# macOS support

go-fuse runs on macOS in two ways: through the macFUSE kernel
extension, or through FSKit. Both need [macFUSE](https://macfuse.github.io/).
FUSE-T is not supported.

The main developer (hanwen@) does not own a Mac, but accepts patches
to make go-fuse work better on macOS. Tests are expected to pass;
report any failure as a bug.

## macFUSE kernel extension

This is the default. `fuse.NewServer` and `fs.Mount` work as on Linux.
They run `mount_macfuse` (or the older `mount_osxfuse`) to mount the
file system.

Limitations:

* The kernel extension speaks protocol version 7.19, so newer
  features are not available.
* There is no splice and no passthrough.
* Requests are read by one goroutine. Concurrent reads from the FUSE
  device race on unmount, which can make unmount hang.
* macOS issues STATFS calls continuously. This costs performance.
* The file system must answer STATFS, or the mount fails. The `fs`
  package returns a zeroed result if the node does not implement
  `NodeStatfser`.
* macFUSE times out operations after 60 seconds by default. go-fuse
  turns this off with `daemon_timeout=0`, to match Linux.
* `Server.Stop` cannot interrupt a reader that waits for a request.
  The reader stops after the next request arrives.
* `LoopbackNode.Mknod` is path based, because Darwin has no `mknodat`.
* Kernel notifications are only tested on Linux.

## FSKit

The `fskit` package mounts through Apple's FSKit, using the user-space
framework in macFUSE 5.3.0 or newer. It does not need the kernel
extension. It loads the framework at run time with
[purego](https://github.com/ebitengine/purego), so it does not need
cgo.

Use `fskit.Mount` for an `fs` tree, or `fskit.MountRaw` for a
`fuse.RawFileSystem`. Use `fskit.Available` to check that the
framework is installed.

Setup:

* Open the macFUSE app once, to install the helper tools and register
  the FSKit extension.
* Enable the extension in System Settings → General → Login Items &
  Extensions → File System Extensions.

The error from `Mount` says which step is missing.

Limitations:

* Kernel notifications are not supported. `EntryNotify`,
  `InodeNotify` and the other notify calls return ENOSYS.
* The mount is served by `fuse.ProtocolServer`, not `fuse.Server`.
  So `Server.Stop`, `Server.Dev` and handing the mount to another
  process are not available.
