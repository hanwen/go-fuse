# Testing against a custom kernel

These scripts build a Linux kernel from a source tree, typically one
with FUSE patches, and run the go-fuse tests against it in QEMU.

Requirements: `qemu-system-x86_64`, `mkfs.ext4`, `curl`, the usual
kernel build tools, and write access to `/dev/kvm`. Without KVM, the
guest is emulated, which is much slower.

## Building

```
kernel/build.bash debug|bench <linux-src> [<name>]
```

* `debug` turns on KASAN, UBSAN, kmemleak, lockdep and other runtime
  checks. Use it for running the tests.
* `bench` turns off debugging, tracing and CPU mitigations. Use it for
  benchmarks.

The kernel is built from `x86_64_defconfig` plus `base.config` and the
flavor's fragment, and has no modules. The script fails if an option in
the fragments doesn't make it into the final config.

The bzImage and its config end up in
`~/.cache/go-fuse-kernel/<flavor>-<name>/`. `<name>` defaults to
`git describe --always --dirty` of the source tree, so kernels of
different versions are kept side by side. Objects are kept per source
tree and flavor under `~/.cache/go-fuse-kernel/obj/`, so rebuilds are
incremental.

## Running

```
kernel/run.bash <kernel> [<pkg> ...] [-- <test flags>]
```

`<kernel>` is a directory from `build.bash`, a bzImage, or a kernel
build directory. Packages are directories relative to the repository
root, and default to all packages with tests except `virtiofs`, which
needs QEMU itself.

Each package's test binary runs inside the guest, as root, against the
guest's `/dev/fuse`. The guest has no `fusermount`, so go-fuse mounts
directly. Its `/tmp` is an ext4 scratch disk.

The run fails if a test fails, if the guest doesn't finish, or if the
kernel reports a problem: a BUG, WARNING, KASAN or UBSAN report, a
lockdep splat, a hung task, a kmemleak leak (scanned at the end of the
run), or a panic. Test output and the guest console are kept in the
output directory, which is printed at the end.

Examples:

```
# All tests on a debug kernel.
kernel/run.bash ~/.cache/go-fuse-kernel/debug-<name>

# One test.
kernel/run.bash ~/.cache/go-fuse-kernel/debug-<name> fs -- -test.run=TestNotifyReaddirplus -test.v

# Benchmarks on an optimized kernel.
kernel/run.bash ~/.cache/go-fuse-kernel/bench-<name> benchmark -- -test.run='^$' -test.bench=.
```

Test flags go to the test binaries, so they take the `-test.` prefix.

| Variable | Default | |
|---|---|---|
| `GO_FUSE_VM_OUT` | a new temporary directory | where to keep the output |
| `GO_FUSE_VM_TIMEOUT` | `1h` | limit for the whole guest run |
| `GO_FUSE_VM_MEM` | `8G` | guest memory |
| `GO_FUSE_VM_DISK` | `64G` | size of the sparse scratch disk |
| `GO_FUSE_VM_APPEND` | | extra kernel command line |

## Interactive use

The kernels also boot under [virtme-ng](https://github.com/arighi/virtme-ng),
which gives a shell in a guest that sees the host's filesystem:

```
vng --force-9p --run ~/.cache/go-fuse-kernel/debug-<name> -m 8G
```
