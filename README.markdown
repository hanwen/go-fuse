GO-FUSE: native bindings for the FUSE kernel module.
====================================================


HIGHLIGHTS
----------

* High speed: less than 50% slower than libfuse, using the gc compiler.  For most real world applications, the difference will be
negligible.

* Supports in-process mounting of different `FileSystems` onto subdirectories of the FUSE mount.

* Supports 3 interfaces for writing filesystems:
	- `PathFileSystem`: define filesystems in terms path names.
	- `NodeFileSystem`: define filesystems in terms of inodes.
	- `RawFileSystem`: define filesystems in terms of FUSE's raw wire protocol.

* Both `NodeFileSystem` and `PathFileSystem` support manipulation of true hardlinks.
  
* Includes two fleshed out examples, zipfs and unionfs.


EXAMPLES
--------

* `examples/hello/hello.go` contains a 60-line "hello world" filesystem

* `zipfs/zipfs.go` contains a small and simple read-only filesystem for zip and tar files. The corresponding command is in `example/zipfs/`. For example,

		mkdir /tmp/mountpoint
		example/zipfs/zipfs /tmp/mountpoint file.zip &
		ls /tmp/mountpoint
		fusermount -u /tmp/mountpoint

* `zipfs/multizipfs.go` shows how to use in-process mounts to combine multiple Go-FUSE filesystems into a larger filesystem.

* `fuse/loopback.go` mounts another piece of the filesystem. Functionally, it is similar to a symlink.  A binary to run is in `example/loopback/`. For example

		mkdir /tmp/mountpoint
		example/loopback/loopback -debug /tmp/mountpoint /some/other/directory &
		ls /tmp/mountpoint
		fusermount -u /tmp/mountpoint

* `unionfs/unionfs.go`: implements a union mount using 1 R/W branch, and multiple R/O branches.

		mkdir -p  /tmp/mountpoint /tmp/writable
		example/unionfs/unionfs /tmp/mountpoint /tmp/writable /usr &
		ls /tmp/mountpoint
		ls -l /tmp/mountpoint/bin/vi
		rm /tmp/mountpoint/bin/vi
		ls -l /tmp/mountpoint/bin/vi
		cat /tmp/writable/*DELETION*/*

* `union/autounionfs.go`: creates UnionFs mounts automatically based on existence of READONLY symlinks.


Tested on:

- x86 32bits (Fedora 14).
- x86 64bits (Ubuntu Lucid).


BENCHMARKS
----------

We use threaded stats over a read-only filesystem for benchmarking. Automated code is under `benchmark/ .sh`. A simple C version of the same FS gives a FUSE baseline

Data points (time per stat, Go-FUSE version Sep 3), using java 1.6 src.zip (7000 files).

	platform               libfuse      Go-FUSE     difference (%)

	Lenovo T60/F15 (2cpu)  106us        125us       18%
	DellT3500/Lucid (2cpu) 33us         42us        27%
	Lenovo T400  (2cpu)    64us         77us        20%


CREDITS
-------

* Inspired by Taru Karttunen's package, https://bitbucket.org/taruti/go-extra.

* Originally based on Ivan Krasin's https://github.com/krasin/go-fuse-zip


BUGS
----

Yes, probably. Report them through golang-nuts@googlegroups.com.


KNOWN PROBLEMS
--------------

Grep source code for TODO.  Major topics:

* Support for umask in Create

* Use splice for transporting data, use io.Reader in API.

* Opendir/Readdir does not support seeking

* Missing support for network FS file locking: `FUSE_GETLK`, `FUSE_SETLK`, `FUSE_SETLKW`

* Missing support for `FUSE_INTERRUPT`, `CUSE`, `BMAP`, `POLL`, `IOCTL`

* In the path API, renames are racy; See also:
  http://sourceforge.net/mailarchive/message.php?msg_id=27550667


LICENSE
-------

Like Go, this library is distributed under the new BSD license.  See accompanying LICENSE file.

