# Complete directory listings: negative lookups from READDIRPLUS

Status: implemented as a three-patch kernel series on branch
`fuse-complete-dir` (on top of 7.3-rc7):

1. `fuse: add a negative lookup cache for directories`
2. `fuse: fill the negative lookup cache from complete READDIRPLUS passes`
3. `fuse: let the server ask for a directory to be read in the background`

The go-fuse side is in this branch. The sections below describe what the
patches do; "Open items" lists what is still missing.

## Goal

After the kernel has read a directory completely with READDIRPLUS, and
the server has marked the directory as eligible, the kernel answers
lookups of names that were *not* in the listing with ENOENT without
sending FUSE_LOOKUP. The listing expires with the `entry_valid` of
`.`, or without one, at the smallest `entry_valid` of its entries.

Negative entries returned by LOOKUP don't solve this: each name still
costs one round trip the first time, and the set of names that don't
exist is unbounded. Typical workloads are search paths (compiler
include paths, shared libraries, interpreters' module paths) and build
systems probing for outputs.

## Why the dcache isn't enough

A negative answer is only valid if the kernel saw **all** of the
directory: a single pass from offset 0 to EOF, with no seeks and no
changes to the directory in the middle. Two existing mechanisms look
reusable but can't be the source of truth:

* **The dcache:** dentries created by READDIRPLUS can be pruned under
  memory pressure, so "no dentry" does not mean "not in the listing".
* **The `rdc` page cache (FOPEN_CACHE_DIR):** reclaim can evict it, and
  it only exists when the server asked for caching.

So the directory inode carries its own record of which names the
listing contained.

## Protocol (`include/uapi/linux/fuse.h`, 7.48)

* **INIT flag `FUSE_HAS_COMPLETE_DIR` (`1ULL << 46`):** the kernel
  supports FOPEN_COMPLETE_DIR and FUSE_NOTIFY_READDIRPLUS. Only
  advertised; the kernel doesn't check whether the server echoes it.
  Bits 44 and 45 are `FUSE_HAS_SYNCFS` and `FUSE_PASSTHROUGH_V2` in
  `for-next`. Detect support by this flag, not the minor version, which
  may be folded back into 7.47.
* **OPENDIR flag `FOPEN_COMPLETE_DIR` (`1 << 9`):** the per-directory
  opt-in, like FOPEN_CACHE_DIR. The READDIRPLUS reply has no header and
  an EOF reply is empty, so a per-reply flag would have nowhere to go.
  Only honored when READDIRPLUS is enabled (`do_readdirplus`).
* **Notification `FUSE_NOTIFY_READDIRPLUS` (12)** (10 and 11 are
  `FUSE_NOTIFY_BACKING_REMOVE`/`_MAP`), payload
  `struct fuse_notify_readdirplus_out { u64 nodeid; u32 flags; u32 padding; }`:
  ask the kernel to read directory `nodeid` in the background, as if it
  was opened with FOPEN_COMPLETE_DIR. See "Background pass" below.

### Server contract

* Setting FOPEN_COMPLETE_DIR asserts that the listing is authoritative
  until the `entry_valid` of `.` expires, if `.` has a nodeid, and
  otherwise until the smallest `entry_valid` of the entries with a
  nodeid expires.
* Entries with nodeid 0 carry no timeout and don't count towards the
  expiry. Their names are still recorded.
* `.` is never looked up and doesn't count as a lookup. Its timeout is
  the natural place for the server's negative timeout, and it is how a
  server covers an empty directory. (go-fuse `fs` does this; see below.)
* If no entry carries a timeout, nothing is cached. A `.` with timeout 0
  means the listing has already expired.
* Every name that LOOKUP resolves must be in the listing byte for byte;
  names only found by eg. case folding are answered as negative.
* If a name is added remotely before the listing expires, the server
  must send `NOTIFY_INVAL_ENTRY(parent, name)`. This is the same
  contract as for negative entries returned by LOOKUP: negative dentries
  created from the listing get its expiry and are only dropped by
  `NOTIFY_INVAL_ENTRY` for their name.
* virtio-fs has no notifications, so a virtio-fs server must only set
  the flag where staleness up to the timeout is acceptable.

## Name set: a Bloom filter (`fs/fuse/dir_names.c`)

The set is only used to prove absence, and a false positive costs just
the LOOKUP that would have been sent anyway, so exact membership isn't
needed.

* `struct fuse_dir_names { rcu_head; expires; epoch; mask; count;
  max_count; bits[] }`, RCU-protected, in `fi->rdc.names`, next to
  `fi->rdc.names_gen` (a generation counter bumped on every local
  modification). Both are protected by `fi->rdc.lock`.
* Hash: `siphash()` with a random per-connection key
  (`fc->names_key`), so a server can't steer names into false positives.
  The 64-bit hash gives two 32-bit halves for double hashing.
* 7 probes; nominally 10 bits per name (about 1% false positives),
  rounded up to a power of two, minimum `BITS_PER_LONG`. 500 names get
  8192 bits, which gives about 0.06% false positives.
* Sized at build time from the number of names in the listing. No
  filter for listings above `FUSE_DIR_NAMES_MAX` (2^18) names.
* `max_count` is twice the nominal capacity. Names added after the
  build (local creates, see below) raise the false-positive rate, so the
  filter is dropped once `count` reaches `max_count`.

## Building the filter (`fs/fuse/readdir.c`)

Each open file has its own builder, `ff->readdir.names`
(`struct fuse_dir_names_builder`), holding a growable array of 64-bit
hashes, the minimum expiry, the epoch, the names generation at the
start, and the `ctx->pos` the next getdents call must have.

* **Start:** at `ctx->pos == 0` when the file was opened with
  FOPEN_COMPLETE_DIR. Samples `names_gen` and the epoch.
* **Force READDIRPLUS:** for every request of the pass, because
  `readdirplus_auto` would otherwise switch to READDIR, which has no
  timeouts.
* **Collect:** in `parse_dirplusfile`, for every entry, including the
  ones past `over` (the user buffer was full but the loop still walks
  them). `.` and `..` contribute to the expiry but not to the names.
* **Discard:** on a seek (`ctx->pos` doesn't match), on an error, when
  the array would exceed `FUSE_DIR_NAMES_MAX`, and in
  `fuse_dir_release`.
* **Publish:** at EOF. Allocate a filter of the right size, insert the
  hashes, and install it with `rcu_replace_pointer` under
  `fi->rdc.lock` if `names_gen` and the epoch are unchanged and the
  expiry is in the future. The old filter is freed with `kvfree_rcu`.
* **FOPEN_CACHE_DIR:** a pass served from the readdir cache sends no
  READDIRPLUS, so no names are collected, and an expired filter isn't
  rebuilt until the cache is.

Readdir holds the directory lock only per request, so a local
modification between requests is detected through `names_gen`.

## Lookup (`fs/fuse/dir.c`)

`fuse_dir_names_absent(dir, name, &expires)`: under `rcu_read_lock`,
true if the filter exists, its epoch is current, it hasn't expired and
the name's bits aren't all set.

* **`fuse_lookup`:** if absent (and the name isn't longer than
  `name_max`), skip `fuse_lookup_name` and splice a negative dentry
  whose time is the filter's expiry. `fuse_atomic_open` gets this for
  free via `fuse_lookup`; the create itself still goes to the server.
* **`fuse_dentry_revalidate`:** unchanged. An expired negative dentry is
  invalidated as before, and the following `->lookup` hits the filter
  again.
* `struct fuse_dentry` gets no generation field, so negative dentries
  created from a filter outlive the filter (see the server contract).

## Keeping the filter current

| Event | Effect |
|---|---|
| Successful CREATE, MKNOD, MKDIR, SYMLINK, LINK | add the name; bump `names_gen` |
| Successful RENAME | add the new name to the target directory |
| Any of the above failing | drop the filter (an interrupted request may have been applied) |
| UNLINK, RMDIR, rename source | nothing: a removed name stays in the filter and is looked up as before; `fuse_dir_changed` bumps `names_gen` |
| `NOTIFY_INVAL_ENTRY` | add the name, **before** looking up the dentry. A remotely added name usually has none, and the old code returned ENOENT before reaching anything. A name in the filter is just looked up, which is right whether it was added, removed or replaced, so this keeps the listing. |
| `NOTIFY_INVAL_INODE` on a directory | drop the filter |
| `NOTIFY_INC_EPOCH` | checked lazily: the filter stores its epoch |
| Inode freed | free the filter in `fuse_free_inode` |

The expiry isn't changed when names are added: it bounds negative
answers, and a new name is looked up again once its own dentry expires.

## Background pass: `FUSE_NOTIFY_READDIRPLUS`

Lets a server that sees negative lookups make them local without
waiting for an application to list the directory.

* **Result:** 0 if a pass was queued. EALREADY if the directory already
  has a valid filter or a pass is queued or running
  (`FUSE_I_READDIRPLUS_HINT`). ENOENT if the kernel has no inode or no
  dentry for the nodeid (eg. after the dcache was shrunk), or the
  superblock is going away. EBUSY if too many directories are being
  read on the connection (currently 8, not part of the ABI; treat like
  EALREADY). ENOTDIR, EOPNOTSUPP (no READDIRPLUS; independent of
  FUSE_PARALLEL_DIROPS), EIO (bad inode), ENOMEM, EINVAL (non-zero
  `flags`/`padding` or wrong size).
* **Usage:** because of EALREADY, a server can send it after every
  LOOKUP it answered negatively, without state of its own. Lookups that
  still reach the server are false positives or removed names, which it
  can't tell apart from lookups in a directory without a listing.
* **Ordering:** it only queues a work item and doesn't wait for the
  server, so it can be sent from inside a request handler, eg. from the
  LOOKUP handler before replying ENOENT.
* **Implementation:** a work item on `system_dfl_long_wq` does a regular
  FUSE_OPENDIR, READDIRPLUS requests from offset 0 to EOF (taking the
  directory lock shared for each one, like getdents), and RELEASEDIR.
  The readdir code works on a `struct fuse_file` and the directory's
  dentry, since there is no `struct file`. FOPEN_CACHE_DIR is cleared
  for the pass, as the readdir cache's validation assumes reads through
  a file. The pass stops as soon as the listing can't be completed.
* **Credentials:** the requests are not made on behalf of any process:
  OPENDIR and READDIRPLUS carry `FUSE_INVALID_UIDGID` (go-fuse
  `fuse.INVALID_UIDGID`) as uid and gid, and pid 0 or a kernel thread's.
  A permission-checking server must allow them. RELEASEDIR carries 0, as
  for every release. The requests are ordinary (not background)
  requests, so they don't count against `max_background`. Each
  READDIRPLUS holds the directory lock shared, so a slow reply holds up
  creates in that directory, as for getdents().
* **Lifetime:** the work item holds an inode reference, and the mount
  counts its passes (`fm->readdir_hints`). `fuse_sb_destroy()` stops new
  passes and waits for the running ones after aborting the connection
  (if this was its last mount), so an unmount doesn't wait for the
  server to answer. Passes are only queued under `fc->killsb`.
  fuseblk sends a synchronous DESTROY before the abort, and no request
  may follow DESTROY, so there the passes are drained first: `umount`
  waits for a blocked READDIRPLUS handler, however many threads the
  server has.

Pushing the listing in the notification was considered and rejected: it
would need new rules for lookup counts and a handshake to detect races
with local modifications. With the kernel doing the reads, lookup counts
and race detection work exactly as for a pass by an application.

## go-fuse changes

Done, in order:

* `fuse, benchmark: add FOPEN_COMPLETE_DIR`: `CAP_HAS_COMPLETE_DIR`,
  `FOPEN_COMPLETE_DIR`, and `BenchmarkNegStatUncachedDir`. Servers opt in with
  `MountOptions.ExtraCapabilities` and by setting the flag in OPENDIR
  replies (`fs.NodeOpendirHandler` or a RawFileSystem wrapper).
* `fs: send "." with nodeid and entry timeout in READDIRPLUS`: `.` gets
  the directory's nodeid and `NegativeTimeout` as its entry timeout, so
  negative answers from a listing last as long as negative LOOKUP
  replies, and empty directories get a listing. `.` doesn't increment
  the lookup count. Without a (non-zero) `NegativeTimeout`, `.` has
  nodeid 0, so the listing lasts for the smallest entry timeout, and an
  empty directory gets none; this is documented on
  `Options.NegativeTimeout`. The default stream for nodes without
  `Readdir` (`childrenAsDirstream`) now yields `.` too; streams from
  `Readdir` must yield it themselves (loopback does).
* `fuse, fs: add FUSE_NOTIFY_READDIRPLUS`:
  `fuse.Server.ReaddirplusNotify(nodeid)` and
  `fs.Inode.NotifyReaddirplus()`; they return ENOSYS unless the kernel
  advertised `CAP_HAS_COMPLETE_DIR`.

To do:

* A way for `fs` to send FUSE_NOTIFY_READDIRPLUS automatically after
  negative LOOKUPs.
* A dedicated `fs.Options`/`MountOptions` switch instead of
  `ExtraCapabilities`, and a way for `fs` nodes to ask for
  FOPEN_COMPLETE_DIR without implementing `OpendirHandle`.

## Testing

### Guest kernel

`kernel/` builds module-less guest kernels, and runs the tests in them:

```
kernel/build.bash debug <linux-src>   # → ~/.cache/go-fuse-kernel/debug-<git describe>/bzImage
kernel/build.bash bench <linux-src>   # → ~/.cache/go-fuse-kernel/bench-<git describe>/bzImage
kernel/run.bash ~/.cache/go-fuse-kernel/debug-<git describe>
kernel/run.bash ~/.cache/go-fuse-kernel/bench-<git describe> benchmark -- -test.run='^$' -test.bench='NegStat|ReadDirEntCompleteDir'
```

`run.bash` runs the test binaries inside the guest, against its
`/dev/fuse` (see `kernel/README.md`). It fails on kernel splats as well
as test failures. The debug kernel has KASAN, UBSAN, kmemleak and
lockdep; the full suite passes on it with the patches applied.

### `fs.TestNotifyReaddirplus`

Runs against `/dev/fuse`, so it needs the patched kernel in the guest
(`kernel/run.bash <kernel> fs -- -test.run=TestNotifyReaddirplus`); elsewhere
it checks for ENOSYS and skips. With a 500-entry directory that was
never listed, it checks that:

* the notification on a file fails with ENOTDIR;
* after a negative LOOKUP, the notification succeeds and the kernel
  sends one OPENDIR, with `INVALID_UIDGID` as uid and gid, then
  READDIRPLUS and RELEASEDIR;
* a second notification fails with EALREADY;
* 100 stats of absent names send at most 2 LOOKUPs, and listed names
  still resolve.

### `fs.TestCompleteDirEmpty`

Runs against `/dev/fuse` and skips unless the kernel advertises
`CAP_HAS_COMPLETE_DIR`
(`kernel/run.bash <kernel> fs -- -test.run=TestCompleteDirEmpty`). It
sets FOPEN_COMPLETE_DIR on every OPENDIR, with one-minute entry and
negative timeouts, and counts LOOKUPs per name. For an empty directory,
where only `.` carries a timeout, it checks that:

* a stat of an absent name before listing sends one LOOKUP;
* after listing, 50 stats of absent names send no LOOKUPs. An empty
  filter has no bits set, so there are no false positives.

### `fs.TestCompleteDirEmptyNoNegativeTimeout`, `fs.TestCompleteDirNegativeTimeout`

Without a `NegativeTimeout`, an empty directory gets no listing: 50
stats of absent names after listing send 50 LOOKUPs. With a 200 ms entry
timeout on a 10-entry directory, the listing still answers absent names
after 400 ms if `NegativeTimeout` is a minute, and has expired without
one.

### `fs.TestCompleteDirLarge`

Lists a 500-entry loopback directory with FOPEN_COMPLETE_DIR and keeps
it open. Doesn't need root. It checks that:

* 50 stats of absent names send at most 2 LOOKUPs;
* a file created in the backing directory stays ENOENT until
  `NOTIFY_INVAL_ENTRY`, then resolves, and absent names are still
  answered from the filter;
* after `NOTIFY_INVAL_INODE` on the directory, a file created in the
  backing directory resolves and every absent name is looked up.

### `fs.TestCompleteDirDropCaches`

Root only (skips otherwise). Same setup; `drop_caches` (2) prunes the
child dentries while the open directory pins its own. It checks that:

* after a drop, all listed names resolve and absent names send at most
  2 LOOKUPs;
* after create, rename and unlink through the mount and another drop,
  the created and renamed names resolve, the old name and the removed
  one are ENOENT, and absent names still send at most 2 LOOKUPs.

All four tests pass on the patched debug kernel in the guest, as does
the full suite.

### Unmount during a background pass

`fs/readdirplus_unmount_linux_test.go`, root only. A READDIRPLUS handler
blocks on a channel while the server keeps reading `/dev/fuse`; the test
sends NOTIFY_READDIRPLUS for directories of 3000 entries, waits for the
OPENDIR and the first READDIRPLUS, and runs `umount` in another process
with a timeout. The server runs with `Debug` and a captured log.

* `TestReaddirplusUnmount/{umount,lazy,abort,dirs}`: plain `umount`,
  `umount -l`, abort through `/sys/fs/fuse/connections/N/abort` first,
  and four directories hinted at once. Each checks that `umount` returns
  within 2 s without releasing the handler, that the device read returns
  ENODEV, that after releasing the handler every READDIRPLUS reply fails
  with ENOENT, that no RELEASEDIR or FORGET arrives, and that after a
  remount a missing name is looked up again.
* `TestReaddirplusUnmountKill`: the server runs in a child process
  (`TestReaddirplusUnmountHelper`), is killed with SIGKILL while the
  handler blocks, and the mount is then unmounted and remounted.
* `TestReaddirplusUnmountFuseblk/single={false,true}`: fuseblk on a
  loop device, with and without `SingleThreaded`. `umount` is still
  running after 2 s and no DESTROY has arrived; once the handler is
  released, the pass ends with RELEASEDIR, then DESTROY, no reply write
  fails, and the block device can be opened with O_EXCL right after.

The guest console has no busy-inode, lockdep, KASAN, WARN or
deprecated-workqueue reports for these. Not covered: a hint followed by
an immediate unmount, looped many times (a race test for `fm` in the
work function), and the drain without an abort, which only virtiofs
submounts reach and virtiofs has no notifications.

### Benchmarks

`benchmark.BenchmarkNegStatUncachedDir` stats unique absent names in a
500-entry directory listed once with FOPEN_COMPLETE_DIR, and reports
`lookups/op`. On the bench kernel in a 4-vCPU KVM guest, on the guest's
`/dev/fuse`, host CPU pinned at 2 GHz, median of 11 runs of
`-test.benchtime=5000x -test.cpu=2`:

| Benchmark | ns/op |
|---|---|
| ReadDirEntCompleteDir (per entry) | 2,281 |
| ReaddirplusHint (per entry) | 1,801 |
| NegStatUncached (LOOKUP round trip) | 22,192 |
| NegStatCached (negative dentry hit) | 797 |
| NegStatUncachedDir (filter) | 1,587 |

Measured on the kernel rebased onto `for-next`, where the hinted read
waits for each reply. `ReaddirplusHint` drops the listing with
NOTIFY_INVAL_INODE and has the kernel read the directory again with
NOTIFY_READDIRPLUS; it is cheaper per entry than getdents(), as no
entries are copied to userspace. The previous kernel, which sent the
hinted read as background requests, gave 2,526 / 25,008 / 786 / 1,806
for the other four.

`lookups/op` is 0.0004, the filter's false-positive rate. The filter
path costs about 0.8 µs more than a negative dentry hit, and a
fourteenth of a LOOKUP round trip. A listing pays off when the
directory has fewer than about 9 entries (11 for a hinted read) per
unique absent name looked up.

### Missing

The `kernel/` infrastructure is submitted, and the tests above run in
the QEMU guest. Still to do:

* The looped hint-then-unmount race test under KASAN.

* The tmpfs reference (unique absent names) on the bench kernel.

## Open items for upstream

* `Documentation/filesystems/fuse/` section for the feature, including
  the server contract.
* Expect the question of why `FUSE_NOTIFY_INVAL_ENTRY` plus negative
  timeouts aren't enough. The answer is that you can't cache a negative
  entry for a name you've never looked up.
* Lifting the filter into the VFS, for other network filesystems with
  expensive lookups, is a possible follow-up.
