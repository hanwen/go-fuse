# Go-FUSE

[![CI](https://github.com/hanwen/go-fuse/actions/workflows/ci.yml/badge.svg)](https://github.com/hanwen/go-fuse/actions/workflows/ci.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/hanwen/go-fuse/v3/fs.svg)](https://pkg.go.dev/github.com/hanwen/go-fuse/v3/fs)

Go native bindings for the FUSE kernel module.

Use
[github.com/hanwen/go-fuse/v3/fs](https://pkg.go.dev/github.com/hanwen/go-fuse/v3/fs)
library.  It follows the wire protocol closely, but provides
convenient abstractions for building both node and path based file
systems

## Release notes

* [v3](docs/v3.md): changes in v3, and how to upgrade from v2.
* [v2](docs/v2.md): v2.5 to v2.12.

## Notable users

Projects that use go-fuse, with their GitHub stars as of November 2026:

| Project | Stars | Use |
|---------|------:|-----|
| [rclone](https://github.com/rclone/rclone) | 61k | `rclone mount` for cloud storage |
| [SeaweedFS](https://github.com/seaweedfs/seaweedfs) | 36k | `weed mount` (through a fork) |
| [Kubo (IPFS)](https://github.com/ipfs/kubo) | 17k | mounting IPFS and IPNS |
| [JuiceFS](https://github.com/juicedata/juicefs) | 15k | distributed POSIX file system (through a fork) |
| [Kopia](https://github.com/kopia/kopia) | 15k | mounting backup snapshots |
| [Pachyderm](https://github.com/pachyderm/pachyderm) | 6k | mounting data repositories |
| [gocryptfs](https://github.com/rfjakob/gocryptfs) | 5k | encrypted overlay file system |
| [Velociraptor](https://github.com/Velocidex/velociraptor) | 4k | mounting collected files |
| [onedriver](https://github.com/jstaf/onedriver) | 2k | Microsoft OneDrive client |
| [Stargz Snapshotter](https://github.com/containerd/stargz-snapshotter) | 2k | lazy pulling of container images |
| [Cloud SQL Auth Proxy](https://github.com/GoogleCloudPlatform/cloud-sql-proxy) | 1k | Unix sockets for database instances |
| [SOCI Snapshotter](https://github.com/awslabs/soci-snapshotter) | 1k | lazy pulling of container images |
| [Buildbarn](https://github.com/buildbarn/bb-remote-execution) | 171 | virtual file system for remote build workers |

## Comparison with other FUSE libraries

See [docs/comparison.md](docs/comparison.md) for a feature comparison
with other FUSE libraries for Go.

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

See [docs/macos.md](docs/macos.md) for macFUSE and FSKit support and
their limitations.

## Windows Support

See [docs/windows.md](docs/windows.md) for mounting on Windows
through WinFsp.

## Credits

* Inspired by Taru Karttunen's package, https://bitbucket.org/taruti/go-extra.

* Originally based on Ivan Krasin's https://github.com/krasin/go-fuse-zip

## Bugs

Report them through https://github.com/hanwen/go-fuse/issues. Please
include a debug trace (set `fuse.MountOptions.Debug` to `true`).

## License

Like Go, this library is distributed under the new BSD license.  See
accompanying LICENSE file.

