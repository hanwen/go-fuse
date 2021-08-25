# GO-FUSE

[![CI](https://github.com/hanwen/go-fuse/actions/workflows/ci.yml/badge.svg)](https://github.com/hanwen/go-fuse/actions/workflows/ci.yml)
[![GoDoc](https://godoc.org/github.com/hanwen/go-fuse?status.svg)](https://godoc.org/github.com/hanwen/go-fuse)

Go native bindings for the FUSE kernel module.

You should import and use
[github.com/hanwen/go-fuse/fs](https://godoc.org/github.com/hanwen/go-fuse/fs)
library.  It follows the wire protocol closely, but provides
convenient abstractions for building both node and path based file
systems

Older, deprecated APIs are available at
[github.com/hanwen/go-fuse/fuse/pathfs](https://godoc.org/github.com/hanwen/go-fuse/fuse/pathfs)
and
[github.com/hanwen/go-fuse/fuse/nodefs](https://godoc.org/github.com/hanwen/go-fuse/fuse/nodefs).

## Comparison with other FUSE libraries

The FUSE library gained a new, cleaned-up API during a rewrite
completed in 2019. Find extensive documentation
[here](https://godoc.org/github.com/hanwen/go-fuse/).

Further highlights of this library is

* Comprehensive and up to date protocol support (up to 7.12.28).

* Performance that is competitive with libfuse.


## Examples

* `example/hello/main.go` contains a 60-line "hello world" filesystem

* `zipfs/zipfs.go` contains a small and simple read-only filesystem for
  zip and tar files. The corresponding command is in example/zipfs/
  For example,

  ```shell
  mkdir /tmp/mountpoint
  example/zipfs/zipfs /tmp/mountpoint file.zip &
  ls /tmp/mountpoint
  fusermount -u /tmp/mountpoint
  ````

* `zipfs/multizipfs.go` shows how to use in-process mounts to
  combine multiple Go-FUSE filesystems into a larger filesystem.

* `fuse/loopback.go` mounts another piece of the filesystem.
  Functionally, it is similar to a symlink.  A binary to run is in
  example/loopback/ . For example

  ```shell
  mkdir /tmp/mountpoint
  example/loopback/loopback -debug /tmp/mountpoint /some/other/directory &
  ls /tmp/mountpoint
  fusermount -u /tmp/mountpoint
  ```

## macOS Support

go-fuse works somewhat on OSX. Known limitations:

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

Yes, probably.  Report them through
https://github.com/hanwen/go-fuse/issues

## Disclaimer

This is not an official Google product.

## Known Problems

Grep source code for TODO.  Major topics:

* Missing support for `CUSE`, `BMAP`, `IOCTL`

## License

Like Go, this library is distributed under the new BSD license.  See
accompanying LICENSE file.

--------

## Appendix I. Go-FUSE log format

To increase signal/noise ratio Go-FUSE uses abbreviations in its debug log
output. Here is how to read it:

- `iX` means `inode X`;
- `gX` means `generation X`;
- `tA` and `tE` means timeout for attributes and directory entry correspondingly;
- `[<off> +<size>)` means data range from `<off>` inclusive till `<off>+<size>` exclusive;
- `Xb` means `X bytes`.

Every line is prefixed with either `rx <unique>` or `tx <unique>` to denote
whether it was for kernel request, which Go-FUSE received, or reply, which
Go-FUSE sent back to kernel.

Example debug log output:

```
rx 2: LOOKUP i1 [".wcfs"] 6b
tx 2:     OK, {i3 g2 tE=1s tA=1s {M040755 SZ=0 L=0 1000:1000 B0*0 i0:3 A 0.000000 M 0.000000 C 0.000000}}
rx 3: LOOKUP i3 ["zurl"] 5b
tx 3:     OK, {i4 g3 tE=1s tA=1s {M0100644 SZ=33 L=1 1000:1000 B0*0 i0:4 A 0.000000 M 0.000000 C 0.000000}}
rx 4: OPEN i4 {O_RDONLY,0x8000}
tx 4:     38=function not implemented, {Fh 0 }
rx 5: READ i4 {Fh 0 [0 +4096)  L 0 RDONLY,0x8000}
tx 5:     OK,  33b data "file:///"...
rx 6: GETATTR i4 {Fh 0}
tx 6:     OK, {tA=1s {M0100644 SZ=33 L=1 1000:1000 B0*0 i0:4 A 0.000000 M 0.000000 C 0.000000}}
rx 7: FLUSH i4 {Fh 0}
tx 7:     OK
rx 8: LOOKUP i1 ["head"] 5b
tx 8:     OK, {i5 g4 tE=1s tA=1s {M040755 SZ=0 L=0 1000:1000 B0*0 i0:5 A 0.000000 M 0.000000 C 0.000000}}
rx 9: LOOKUP i5 ["bigfile"] 8b
tx 9:     OK, {i6 g5 tE=1s tA=1s {M040755 SZ=0 L=0 1000:1000 B0*0 i0:6 A 0.000000 M 0.000000 C 0.000000}}
rx 10: FLUSH i4 {Fh 0}
tx 10:     OK
rx 11: GETATTR i1 {Fh 0}
tx 11:     OK, {tA=1s {M040755 SZ=0 L=1 1000:1000 B0*0 i0:1 A 0.000000 M 0.000000 C 0.000000}}
```
