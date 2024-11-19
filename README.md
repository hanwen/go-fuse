# Go-FUSE

[![CI](https://github.com/hanwen/go-fuse/actions/workflows/ci.yml/badge.svg)](https://github.com/hanwen/go-fuse/actions/workflows/ci.yml)
[![GoDoc](https://godoc.org/github.com/hanwen/go-fuse/v2/fs?status.svg)](https://godoc.org/github.com/hanwen/go-fuse/v2/fs)

Go native bindings for the FUSE kernel module.

Use
[github.com/hanwen/go-fuse/v2/fs](https://godoc.org/github.com/hanwen/go-fuse/v2/fs)
library.  It follows the wire protocol closely, but provides
convenient abstractions for building both node and path based file
systems

## Release notes

v2.7

* fuse, fs: support STATX

v2.6

* general:
  * drop support for go1.16 
* fuse:
  * FreeBSD support
  * passthrough support for increased performance
  * DirEntryList.Offset and DirEntry.Off are visible now; add DirEntry.Parse 
* fs:
  * new directory API, supporting caching and file handles for Readdir and FsyncDir 
  * passthrough support for increased performance
  * allow LoopbackNode to be used as non-root
  * OnForget method

v2.5

* Support for RenameExchange on Darwin


## Comparison with other FUSE libraries

Further highlights of this library is

* Comprehensive and up to date protocol support (up to 7.12.28).

* Performance that is competitive with libfuse.


## Examples

* [example/hello/](example/hello/main.go) contains a 60-line "hello world" filesystem

* [zipfs/zipfs](zipfs/zipfs.go) contains a small and simple read-only filesystem for
  zip and tar files. The corresponding command is in example/zipfs/
  For example,

  ```shell
  mkdir /tmp/mountpoint
  example/zipfs/zipfs /tmp/mountpoint file.zip &
  ls /tmp/mountpoint
  fusermount -u /tmp/mountpoint
  ````

* [zipfs/multizipfs](zipfs/multizipfs.go) shows how to use combine
  simple Go-FUSE filesystems into a larger filesystem.

* [example/loopback](example/loopback/main.go) mounts another piece of the filesystem.
  Functionally, it is similar to a symlink.  A binary to run is in
  example/loopback/ . For example

  ```shell
  mkdir /tmp/mountpoint
  example/loopback/loopback -debug /tmp/mountpoint /some/other/directory &
  ls /tmp/mountpoint
  fusermount -u /tmp/mountpoint
  ```

## macOS Support

The main developer (hanwen@) does not own a Mac to test, but accepts
patches to make Go-FUSE work on Mac.

* All of the limitations of OSXFUSE, including lack of support for
  NOTIFY.

* OSX issues STATFS calls continuously (leading to performance
  concerns).

* OSX has trouble with concurrent reads from the FUSE device, leading
  to performance concerns.

* Tests are expected to pass; report any failure as a bug!

## Credits

* Inspired by Taru Karttunen's package, https://bitbucket.org/taruti/go-extra.

* Originally based on Ivan Krasin's https://github.com/krasin/go-fuse-zip

## Bugs

Report them through https://github.com/hanwen/go-fuse/issues. Please
include a debug trace (set `fuse.MountOptions.Debug` to `true`).

## License

Like Go, this library is distributed under the new BSD license.  See
accompanying LICENSE file.

