# Comparison with other Go FUSE libraries

This document compares go-fuse with other FUSE libraries for Go. The
facts about other libraries come from their documentation as of
November 2026. A "?" means the documentation does not say.

## Libraries

* [bazil.org/fuse](https://github.com/bazil/fuse) is a pure Go
  implementation of the kernel protocol. It supports Linux and FreeBSD.
  It dropped macOS support when OSXFUSE stopped being open source.
  Its last tagged version on pkg.go.dev is from January 2023.
* [anacrolix/fuse](https://github.com/anacrolix/fuse) is a fork of
  bazil.org/fuse. It adds macOS support through macFUSE and FUSE-T.
* [jacobsa/fuse](https://github.com/jacobsa/fuse) is a pure Go
  implementation that started from the bazil.org/fuse kernel code. It
  supports Linux and macOS (FUSE-T and macFUSE). Google Cloud Storage
  FUSE (gcsfuse) uses it.
* [winfsp/cgofuse](https://github.com/winfsp/cgofuse) wraps the C
  libraries libfuse, macFUSE and WinFsp with cgo. On Windows it
  loads WinFsp without cgo. rclone uses it.
* [rsc/fuse](https://code.google.com/archive/p/rsc/) is
  an early library by Russ Cox, from around 2011. It is described
  below under "History".
* [RogueTeam/openfuse](https://github.com/RogueTeam/openfuse) is a
  pure Go library for OpenBSD only. It mounts an `io/fs.FS`.

## Features

|                              | go-fuse | bazil | jacobsa | cgofuse | openfuse |
|------------------------------|---------|-------|---------|---------|----------|
| Pure Go (no cgo)             | yes     | yes   | yes     | no (except Windows) | yes |
| Linux                        | yes     | yes   | yes     | yes     | no       |
| macOS                        | macFUSE, FSKit | no | macFUSE, FUSE-T | macFUSE | no |
| FreeBSD                      | yes     | yes   | ?       | yes     | no       |
| OpenBSD                      | yes     | no    | no      | yes     | yes      |
| Windows                      | WinFsp  | no    | no      | WinFsp  | no       |
| Raw protocol API             | yes     | yes   | yes (ops) | no    | no       |
| Inode-based API              | yes     | yes   | no      | no      | no       |
| Path-based API               | no      | no    | no      | yes     | yes (`fs.FS`) |
| Cache invalidation (notify)  | yes     | yes   | yes     | Windows only | ?        |
| Store/retrieve page cache    | yes     | yes   | no      | no      | ?        |
| Splice for reads             | yes     | ?     | ?       | ?       | ?        |
| Passthrough                  | yes     | ?     | ?       | ?       | ?        |
| virtiofs (vhost-user) server | yes     | no    | no      | no      | no       |
| Hand mount to new process    | yes     | no    | no      | no      | no       |
| Interrupts as `context`      | yes     | yes   | yes     | no      | ?        |
| License                      | BSD-3   | BSD   | Apache-2.0 | MIT  | BSD-3    |

## History

Russ Cox wrote an early Go FUSE library, `code.google.com/p/rsc/fuse`,
which is the common ancestor for bazil.org/fuse, anacrolix/fuse and
jacobsa/fuse.

go-fuse is a separate code base. Its first version was announced in
December 2010.

## go-fuse features

* Two APIs. The `fs` package manages the inode tree for you. The
  `fuse` package exposes the raw protocol through `RawFileSystem`.
* Full support for kernel notifications, including page cache
  store/retrieve and pruning inodes.
* Zero-copy reads through splice, and FUSE passthrough to backing
  files on Linux.
* `fs.LoopbackRoot` for loopback file systems that are safe against
  symlink races.
* NFS export support, ID-mapped mounts, `STATX`, `IOCTL`, `O_TMPFILE`.
* Serving to virtual machines over vhost-user (`virtiofs`), to
  macOS FSKit (`fskit`), and to Windows through WinFsp (`winfsp`),
  without cgo.
* Handing a live mount to another process, for upgrades without
  unmounting (`Server.Dev`, `NewServerFromDev`).
* POSIX conformance tests (`posixtest`) that run on every supported
  platform.
