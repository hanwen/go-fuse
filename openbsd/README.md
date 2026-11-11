# OpenBSD

OpenBSD's fuse(4) speaks the Linux FUSE wire protocol since mid-2026,
so go-fuse supports it from OpenBSD 8.0 snapshots onwards. Earlier
releases use a different, OpenBSD-specific protocol and do not work.

The kernel implements a small subset of FUSE: protocol version 7.19,
with no capability flags. File systems built on the `fs` package pass
`posixtest` through the `fs` TestPosix, apart from the tests for
features listed below. References are to `sys/miscfs/fuse/` in the
OpenBSD source.

## Mounting

There is no mount helper. go-fuse opens the first free `/dev/fuseN`
and calls mount(2) directly:

* Each `/dev/fuseN` serves one mount at a time; a second open fails
  with EBUSY. MAKEDEV creates few units; make more with
  `mknod /dev/fuseN c <major> N`.
* `MountOptions.Options` understands `ro`, `rw`, `noatime`, `nosuid`,
  `nodev`, `noexec`, `sync` and `async`. Other options, such as
  `default_permissions` or `subtype`, are ignored.
* Only the user who mounted the file system can access it, unless
  `AllowOther` is set, which only root may do. Other users get EACCES,
  EPERM from statfs(2), and made-up attributes for the mount point
  from stat(2).
* go-fuse passes `MaxWrite` as the kernel's `max_read`; it caps reads,
  writes and readdir buffers. The kernel limit is 4 MiB.
* We have only tested mounting as root.

Unmounting sends FUSE_DESTROY and waits for the reply, after which
reads from the device return ENODEV and the server exits.

## Kernel limitations

### No caching of lookups or attributes

fuse(4) never uses the name cache, and ignores `entry_valid` and
`attr_valid` in replies (`fusebuf.h` marks them "Not implemented").
Every path component is looked up with FUSE_LOOKUP, and attributes are
fetched with FUSE_GETATTR whenever the kernel needs them. As a result,
`EntryTimeout` and `AttrTimeout` have no effect.

A negative entry (a LOOKUP reply with NodeId 0) is treated as ENOENT,
but `fuse_lookup.c` returns that ENOENT before the code that lets a
create or rename of a missing name go ahead. Creating a name after a
negative entry therefore fails. `fs.Mount` refuses a positive
`NegativeTimeout` on OpenBSD, and the bridge answers ENOENT when it is
zero.

### Creating files

There is no FUSE_CREATE. creat(2) and open(2) with O_CREAT send
FUSE_MKNOD with S_IFREG, followed by FUSE_OPEN. The `fs` bridge serves
such a MKNOD with `NodeCreater` if the node has no `NodeMknoder`, and
`LoopbackNode.Mknod` creates regular files with open(2), because
OpenBSD's mknod(2) only makes devices and FIFOs.

### File handles

The kernel keeps at most one FUSE file handle per vnode for each of
read-only, write-only and read-write, and shares it between all opens
with that mode. OPEN is only sent for the first open of each mode.
Closing a writable descriptor sends FUSE_FLUSH; FUSE_RELEASE is only
sent when the vnode becomes inactive, with open flags guessed from the
handle's mode. Reads use the read-only handle, writes the write-only
one, falling back to the read-write handle.

Similarly, FUSE_FORGET is only sent when a vnode is reclaimed, with all
lookups accumulated, not when a file is removed.

### Permissions

There is no FUSE_ACCESS. The kernel checks permissions itself against
the attributes from FUSE_GETATTR, as with `default_permissions` on
Linux.

### Locks

fcntl(2) and flock(2) locks are handled by the kernel (`lf_advlock`)
and never reach the file system. As usual for POSIX locks, locks taken
by one process never conflict with each other.

### Requests cannot be interrupted

There is no FUSE_INTERRUPT. A process waiting for the file system
sleeps uninterruptibly until it gets a reply, or until the server
closes the device, for example because it exits.

### Notifications

Writes to the device with a zero `unique` are ignored, so the
`Inode.Notify*` methods and `fuse.Server` notifications have no effect.

### Operations the kernel never sends

* FUSE_SETXATTR, FUSE_GETXATTR, FUSE_LISTXATTR, FUSE_REMOVEXATTR:
  OpenBSD has no extended attributes.
* FUSE_IOCTL: ioctl(2) on a FUSE file returns ENOTTY.
* FUSE_POLL: poll(2) and kqueue(2) are handled by the kernel.
* FUSE_FALLOCATE, FUSE_LSEEK, FUSE_COPY_FILE_RANGE: OpenBSD has no
  fallocate, SEEK_HOLE/SEEK_DATA or copy_file_range.
* FUSE_RENAME2, FUSE_TMPFILE, FUSE_READDIRPLUS, FUSE_BMAP.
* FUSE_FSYNCDIR: fsync(2) on a directory does nothing. fdatasync(2)
  sends a plain FUSE_FSYNC.
* FUSE_SETLK, FUSE_GETLK, FUSE_SETLKW: see Locks.

### Other behavior

* Writes to a file opened with O_APPEND get their offset from a
  FUSE_GETATTR just before the write.
* Reads continue until the file system returns less data than asked
  for; the file size is not used to stop.
* `VOP_STRATEGY` does nothing, so mmap(2) of a FUSE file does not get
  its data through the file system.
* There is no O_DIRECT, and no FOPEN_* reply flags are honored.
* A reply must have exactly the size the kernel expects for the
  opcode; anything else fails with EINVAL. The `fuse` package takes
  care of this.

## Testing

`test.bash` in this directory installs the latest snapshot from the
install ISO into a cached disk image on its first run, then runs the
`fs` TestPosix in a QEMU VM. See the comment at the top of the script.
