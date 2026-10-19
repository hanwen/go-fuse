# Complete directory listings: negative lookups from READDIRPLUS

Status: implemented (uncommitted) on branch `fuse-complete-dir` in
`~/vc/linux` (7.3-rc). Differences from the original design are marked
**v1**.

## Goal

After the kernel has read a directory completely with READDIRPLUS, and
the server has marked the directory as eligible, the kernel answers
lookups of names that were *not* in the listing with ENOENT without
sending FUSE_LOOKUP. The listing expires at the smallest `entry_valid`
of its entries.

## Why the dcache isn't enough

A negative answer is only valid if the kernel saw **all** of the
directory: a single pass from offset 0 to EOF, with no seeks and no
changes to the directory in the middle. Two existing mechanisms look
reusable but can't be the source of truth:

* **The dcache:** dentries created by READDIRPLUS can be pruned under
  memory pressure, so "no dentry" does not mean "not in the listing".
* **The `rdc` page cache (FOPEN_CACHE_DIR):** reclaim can evict it, and
  it only exists when the server asked for caching.

So the directory inode needs its own small record of which names the
listing contained.

## Protocol (`include/uapi/linux/fuse.h`, 7.47)

* **INIT flag `FUSE_HAS_COMPLETE_DIR` (`1ULL << 44`):** the kernel
  advertises it, and the server echoes it back to opt in.
* **OPENDIR flag `FOPEN_COMPLETE_DIR` (`1 << 9`):** sets the policy per
  directory, like FOPEN_CACHE_DIR. The READDIRPLUS reply has no header
  and an EOF reply is empty, so a per-reply flag would have nowhere to
  go.
* **Server obligations, to be documented:**
  * By setting the flag, the server asserts the listing is
    authoritative until the smallest `entry_valid` in it expires.
  * If a name is added remotely before then, the server must send
    `NOTIFY_INVAL_ENTRY(parent, name)` or `NOTIFY_INVAL_INODE(dir)`.
  * virtio-fs has no notifications, so a virtio-fs server must only set
    the flag where staleness up to the timeout is acceptable.

## Name set: store only hashes

For each name in the listing, store a 64-bit hash (`hashlen_string()`,
fixed salt) in a sorted array. Lookup of `X` in `D`:

1. **VFS dcache.** If a valid dentry for `X` exists, the VFS answers
   from it and `->lookup` is never called.
2. **`fuse_lookup`.** Runs only when there is no usable dentry (it was
   pruned, never created for a `nodeid == 0` entry, invalidated, or `X`
   doesn't exist). Then:
   * **`hash(X)` is not in the set:** answer ENOENT locally. Every
     listed name's hash was inserted, so this answer is exact.
   * **`hash(X)` is in the set:** either `X` was listed, or it collides
     with another listed name. Send a normal FUSE_LOOKUP and let the
     server decide.

The set is a one-sided filter (a k=1 Bloom filter that stores the full
hash). Collisions cost one extra LOOKUP and never affect correctness.
The cost is 8 bytes per entry, with a cap such as
`max_complete_dir_entries` above which no set is built. A real Bloom
filter (about 10 bits per entry, about 1% false positives) is a
possible later optimization.

## Kernel changes

### 1. State (`fs/fuse/fuse_i.h`, the directory-only `rdc` struct of the union)

```c
struct fuse_dir_names __rcu *complete; /* NULL = no complete listing */
u64 complete_gen;                      /* bumped on every invalidation */
```

`struct fuse_dir_names { struct rcu_head rcu; u64 expires; int epoch; u32 n; u64 hash[]; }`

### 2. Building the set (`fs/fuse/readdir.c`)

Each open file gets its own builder, `ff->readdir.complete_builder`.

* **Start:** in `fuse_readdir_uncached` at `ctx->pos == 0` when
  `FOPEN_COMPLETE_DIR` is set. Allocate the builder and record
  `start_gen = fi->complete_gen` and the epoch.
* **Force READDIRPLUS:** `fuse_use_readdirplus()` must return true for
  every chunk of the pass, because `readdirplus_auto` would otherwise
  switch later chunks to plain READDIR, which carries no timeouts.
* **Collect:** in `parse_dirplusfile`, hash every entry except `.` and
  `..`, including the entries past `over` (the user buffer was full but
  the loop still walks them).
  * For entries with `nodeid != 0`, compute the expiry with
    `fuse_time_to_jiffies(entry_valid, …)` at reply time and track the
    minimum.
  * For `nodeid == 0` entries, add the name but leave the timeout
    alone.
* **Discard:** throw the builder away on a seek (`ctx->pos` doesn't
  match the builder's position) or when the pass reaches the cached
  path.
* **Publish:** on EOF (`res == 0`), take `fi->rdc.lock`. If
  `complete_gen == start_gen`, the epoch matches and the expiry is in
  the future, sort the hashes and swap them in with
  `rcu_assign_pointer`, freeing the old set with `kfree_rcu`.
  Otherwise drop the builder. A minimum `entry_valid` of 0 means don't
  publish.
* **Expiry:** only entries with `nodeid != 0` contribute. If none do (an
  empty directory, or one where every entry has nodeid 0), the set isn't
  published.
* **Clean up:** free the builder in `fuse_dir_release`.

### 3. Lookup fast path (`fs/fuse/dir.c`)

`fuse_dir_complete_absent(dir, name)`: under `rcu_read_lock`, true if
the set exists, has not expired, the epoch matches, and the hash is not
found (binary search).

* **`fuse_lookup`**, before `fuse_lookup_name`: if absent, call
  `d_splice_alias(NULL, entry)` and `fuse_dentry_set_epoch`, and set
  the dentry time to `complete->expires`. No request is sent.
  `fuse_atomic_open` → `fuse_lookup` gets this for free, and the create
  itself still goes to the server.
* **v1:** `fuse_dentry_revalidate` is unchanged. An expired negative
  dentry is invalidated as before, and the following `->lookup` hits the
  fast path, so no request is sent.
* **v1:** `struct fuse_dentry` gets no generation field. Negative
  dentries already created from a set survive the set's invalidation
  until they expire, like server-supplied negative timeouts. To make a
  name appear before then, the server must send `NOTIFY_INVAL_ENTRY` for
  that name; `NOTIFY_INVAL_INODE` only stops new negative answers.

### 4. Invalidation: `fuse_dir_complete_invalidate(dir)`

The helper bumps `complete_gen`, sets the pointer to NULL and frees the
old set via RCU. Call it from:

| Trigger | Where |
|---|---|
| Local create/mkdir/mknod/symlink/link/unlink/rmdir/rename (both dirs) | `fuse_dir_changed()` |
| `NOTIFY_INVAL_ENTRY` | `fuse_reverse_inval_entry`, **before** `d_lookup`. Today it returns `-ENOENT` without calling `fuse_dir_changed` when there is no dentry, which is exactly the "new remote name" case. |
| `NOTIFY_INVAL_INODE` on a directory | `fuse_reverse_inval_inode` |
| `NOTIFY_INC_EPOCH` | checked lazily: the epoch is stored in the set |
| Directory mtime changes after a getattr | compare against an mtime recorded in the set |
| Bad inode, eviction | free in `fuse_evict_inode` |

### 5. Edge cases

* **Concurrent passes:** two open files each have their own builder,
  and the generation check rejects one that raced with a modification.
* **Locking:** without `parallel_dirops`, `fuse_lock_inode` already
  serializes lookup against readdir. With it, the generation check
  covers the race.
* **FOPEN_CACHE_DIR:** a pass served from `rdc` sends no READDIRPLUS,
  so the set isn't rebuilt. The set built on the first uncached pass
  stays valid until it expires. A later improvement is to build it from
  the cache-fill pass too.

### 6. Upstream series

1. uapi: flags and minor version bump.
2. Name set, building, publishing.
3. Lookup and revalidate fast path.
4. Invalidation hooks, including the `fuse_reverse_inval_entry`
   ordering fix.
5. Documentation in `Documentation/filesystems/fuse/`.

Expect the question of why `FUSE_NOTIFY_INVAL_ENTRY` plus negative
timeouts aren't enough. The answer is that you can't cache a negative
entry for a name you've never looked up.

## go-fuse changes

1. **`fuse/types.go`, `fuse/print.go`:** add `CAP_HAS_COMPLETE_DIR` and
   `FOPEN_COMPLETE_DIR`. Opt in through
   `MountOptions.ExtraCapabilities` for now; later add a dedicated
   option.
2. **`fs/`:** filesystems can already return `FOPEN_COMPLETE_DIR` from
   `OpendirHandle`. Keep it independent of the `CACHE_DIR|KEEP_CACHE`
   coupling in `bridge.go`.
3. **Server contract:** `Inode.AddChild` on a directory that was listed
   with the flag must send `NOTIFY_INVAL_ENTRY`.

## Testing

### Guest kernel

`virtiofs/kernel/` has a config fragment and a build script that
builds a bzImage with everything built in (`CONFIG_MODULES=n`):

```
virtiofs/kernel/build.bash ~/vc/linux      # → ~/.cache/go-fuse-virtiofs/linux/arch/x86/boot/bzImage
GO_FUSE_VIRTIOFS_KERNEL=~/.cache/go-fuse-virtiofs/linux/arch/x86/boot/bzImage \
  go test -v -count 1 ./virtiofs/
```

When `GO_FUSE_VIRTIOFS_KERNEL` is set, the tests boot that kernel and
load no modules. Otherwise they boot the host kernel and modules, as
before.

### `virtiofs.TestCompleteDir`

The test wraps the RawFileSystem to count LOOKUPs per name and to add
`FOPEN_COMPLETE_DIR` to OPENDIR replies. It uses a long entry timeout
and no negative timeout, so the first lookup of any name must reach the
server unless the new code answers it.

The guest:

1. `cd /mnt/dir` (pins the directory inode) and `ls`.
2. `stat missing-1` … `missing-N`, expecting ENOENT.
3. `echo 2 > /proc/sys/vm/drop_caches` to prune the child dentries.
4. `stat` the existing names. They must succeed: their hash is in the
   set, so the lookup falls back to the server.
5. `stat missing-after-drop`, expecting ENOENT. The set lives on the
   inode, not in the dentries.
6. `touch new; stat new`: a local create must invalidate the set.

The host asserts:

* If the kernel negotiated `HAS_COMPLETE_DIR`: zero LOOKUPs for
  `missing-*`, at least one LOOKUP for the existing names after the
  drop, and correct exit codes for every guest command.
* Otherwise: only the exit codes, and the test logs that the feature is
  absent.

Notification-driven invalidation can't be tested over virtio-fs.
Test it with a `/dev/fuse` mount in `fs/` instead.
