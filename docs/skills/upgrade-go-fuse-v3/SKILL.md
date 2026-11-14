---
name: upgrade-go-fuse-v3
description: Upgrade a Go file system from github.com/hanwen/go-fuse/v2 to v3. Use when a module imports go-fuse/v2 and must move to v3, or when code fails to build against go-fuse v3.
---

# Upgrade go-fuse v2 to v3

Follow these steps in order. Build after each step. See `docs/v3.md`
in the go-fuse repository for the full list of changes.

## 1. Update the module

1. Check that `go.mod` says `go 1.25` or newer. Raise it if needed.
2. Replace the import path in all `.go` files:
   ```
   grep -rl 'github.com/hanwen/go-fuse/v2' --include=*.go . \
     | xargs sed -i 's|github.com/hanwen/go-fuse/v2|github.com/hanwen/go-fuse/v3|g'
   ```
3. Run `go get github.com/hanwen/go-fuse/v3@latest` and `go mod tidy`.
4. Run `go build ./...`. Fix the errors with the steps below.

## 2. Remove nodefs and pathfs

If the code imports `fuse/nodefs` or `fuse/pathfs`, port it to the `fs`
package. These packages no longer exist. This is a rewrite, not a
rename:

* A `pathfs.FileSystem` becomes a tree of `fs.InodeEmbedder` nodes.
  Implement the `fs.NodeXxxer` interfaces on the node type.
* A `nodefs.File` becomes an `fs.FileHandle` that implements the
  `fs.FileXxxer` interfaces.
* `fuse.Status` results become `syscall.Errno`. Use `fs.OK` for
  success and `fs.ToErrno(err)` to convert errors.
* Mount with `fs.Mount(dir, root, &fs.Options{...})`.
* For a loopback over a directory, use `fs.NewLoopbackRoot`.

Ask the user before starting this rewrite if it is large.

## 3. Fix context use

These changes apply to both the `fs` and the `fuse` APIs.

* Replace `ctx.(*fuse.Context)` casts. Use
  `caller, ok := fuse.FromContext(ctx)`.
* `fuse.FromContext` now returns `fuse.Caller`, not `*fuse.Caller`.
  Drop pointer dereferences, and do not compare the result with `nil`.
  Check `ok` instead.
* `fuse.NewContext(ctx, &caller)` becomes `fuse.NewContext(ctx, caller)`.
* `&fuse.Context{Caller: c, Cancel: ch}` in tests becomes
  `fuse.NewContext(context.Background(), c)`.
* For request details that the input structs lack, use
  `call, ok := fuse.CallFromContext(ctx)`, then `call.KillSuidgid()`,
  `call.SuppGroups()` or `call.SecCtx()`.

## 4. Fix RawFileSystem implementations

Skip this step if the code only uses the `fs` package.

1. In every `RawFileSystem` method, change the first parameter
   `cancel <-chan struct{}` to `ctx context.Context`. Replace
   `<-cancel` with `<-ctx.Done()`. Add the `context` import.
2. Change `ReleaseDir(input *fuse.ReleaseIn)` to
   `ReleaseDir(ctx context.Context, input *fuse.ReleaseIn)`.
3. Rename `Init(server *fuse.Server)` to `OnConnect(conn *fuse.Conn)`.
   Store `conn` instead of the server, if notifications are needed.
4. Replace notification calls on the stored server or
   `ProtocolServer` with `conn.Notify()`:

   | v2                                   | v3                               |
   |--------------------------------------|----------------------------------|
   | `InodeNotify(node, off, len)`        | `Notify().InvalInode(node, off, len)` |
   | `EntryNotify(parent, name)`          | `Notify().InvalEntry(parent, name)` |
   | `DeleteNotify(parent, child, name)`  | `Notify().Delete(parent, child, name)` |
   | `PruneNotify(nodes)`                 | `Notify().Prune(nodes)`          |
   | `InodeNotifyStoreCache(node, off, data)` | `Notify().Store(node, off, data)` |
   | `InodeRetrieveCache(node, off, dest)`    | `Notify().Retrieve(node, off, dest)` |

   The methods on `*fuse.Server` still exist. Keep them if the code
   holds a `*fuse.Server` from `NewServer`.
5. Remove `KernelSettings().SupportsNotify(fuse.NOTIFY_*)` checks.
   Call the notification and treat `fuse.ENOSYS` as unsupported. If
   the check must happen before the call, use
   `SupportsVersion(7, minor)`, but that only covers the kernel, not
   the transport:

   | v2 constant                                   | minor |
   |-----------------------------------------------|-------|
   | `NOTIFY_INVAL_INODE`, `NOTIFY_INVAL_ENTRY`    | 12    |
   | `NOTIFY_STORE_CACHE`, `NOTIFY_RETRIEVE_CACHE` | 15    |
   | `NOTIFY_DELETE`                               | 18    |

6. Remove `SetDebug` methods. Remove calls to `server.SetDebug`. Set
   `MountOptions.Debug` before mounting instead.
7. Remove calls to `server.RecordLatencies` and uses of
   `fuse.LatencyMap`.
8. If the code embeds `fuse.NewDefaultRawFileSystem()`, the new
   `Init(*InitIn, *InitOut)` and `OnConnect(*Conn)` methods come from
   the default. Do not add empty versions.

## 5. Check behavior changes

These compile, but may change behavior. Read the code for each one.

* A type that embeds `fs.LoopbackNode` and overrides `OnForget` must
  call `n.LoopbackNode.OnForget()`. Otherwise the root descriptor
  leaks.
* Code that waits for `Serve` or `Wait` to return after an error now
  sees the whole server close on a read error.
* A file system that relies on `Nlink == 0` for non-directories now
  sees the number of parents.
* Code that adds one directory under two parents is wrong. Use
  `MvChild`, or `RmChild` before `AddChild`.

## 6. Verify

1. Run `gofmt -l .` and `go vet ./...`.
2. Run `go build ./...`, and the cross-builds the project uses.
3. Run the tests. Tests that mount need `/dev/fuse` and `fusermount3`.
4. Search for leftovers. Each command should print nothing:
   ```
   grep -rn 'go-fuse/v2' --include=*.go --include=go.mod .
   grep -rn 'fuse\.Context\b\|cancel <-chan struct{}\|SetDebug\|LatencyMap\|RecordLatencies' --include=*.go .
   grep -rn 'fuse/nodefs\|fuse/pathfs' --include=*.go .
   ```
5. Report what changed, and list any behavior changes from step 5
   that apply.
