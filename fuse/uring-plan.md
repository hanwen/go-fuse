# FUSE-over-io_uring support plan

Target: optional `io_uring` transport for the `fuse/` package on Linux ≥6.10,
gated on kernel advertising `FUSE_OVER_IO_URING` during INIT.

## Architectural seam

`ProtocolServer.HandleRequest(in [][]byte, out [][]byte)` in
`fuse/protocol-server.go:183` is already the transport-agnostic bridge: it
takes virtiofs-shaped iov pairs (`{header, in-args, payload}` /
`{out-header, out-args, payload}`), drives the `RawFileSystem`, and returns
the bytes written. virtiofs already uses it (`virtiofs/virtiofs.go:34`). The
uring transport plugs in at the same seam — no protocol-layer changes.

What stays on `/dev/fuse`:

- INIT handshake (the uring capability is negotiated there).
- Notifications and notify-retrieve replies (`Server.writev`,
  `server.go:718`).
- ENODEV / unmount detection.

What moves to uring rings: every request/reply after INIT, once negotiated.

## Phases

### Phase 1 — negotiation and option plumbing

- Add `MountOptions.EnableIoUring bool` (default false).
- In `handleInit` (`server.go:559`), if the option is set, include
  `FUSE_OVER_IO_URING` (bit 41) in the requested capability mask and check
  the kernel's ack.
- If negotiated:
  - Force `DisableSplice=true` (same reason virtiofs does at
    `protocol-server.go:158`: no fd to splice the reply into; see "Splice
    incompatibility" below).
  - Hand off request traffic from `Server.loop` to the uring transport.
  - Keep one `/dev/fuse` reader goroutine alive for notifications, retrieve
    replies, and ENODEV.
- If not negotiated: behave exactly as today.

### Phase 2 — io_uring transport (`fuse/uring_linux.go`, new file)

Syscall layer:
- `io_uring_setup`, `io_uring_enter`, `io_uring_register` via
  `golang.org/x/sys/unix` where available, falling back to raw `Syscall6`.
- No cgo.

FUSE uring ABI mirrors:
- `struct fuse_uring_req_header`, `struct fuse_uring_ent`, and the
  `FUSE_IO_URING_CMD_{REGISTER,COMMIT_AND_FETCH}` cmd_op constants. Mirror
  the kernel layout in Go and treat the mmap'd entry as fixed offsets.

Per-CPU ring lifecycle:
- One ring per CPU, N entries each (configurable; default tracking the
  kernel's per-entry payload cap, which currently bounds `MaxWrite`).
- Allocate + register the entry buffer region; mmap it.
- On ring startup: submit N `FUSE_URING_REQ_FETCH` SQEs.
- Goroutine per ring, pinned with `runtime.LockOSThread` +
  `sched_setaffinity` to its qid's CPU. This is **required for
  correctness**, not just perf: `io_uring_cmd_complete_in_task` defers
  CQE dispatch onto the submitting task, so a goroutine migrating
  between SQE submit and the deferred completion runs that completion
  on a CPU the kernel did not expect — racy under load and the cause
  of non-deterministic hangs we hit during posixtest.

Hot path per CQE:
1. Slice the entry's mmap region into the `[][]byte` shape
   `HandleRequest` expects.
2. Call `ps.HandleRequest(inIOV, outIOV)`.
3. Submit `FUSE_URING_REQ_COMMIT_AND_FETCH` on the same entry to publish
   the reply and re-arm the slot.

Interrupts and inflight tracking already live on the protocol layer
(`protocol-server.go:113`) and work unchanged.

### Phase 3 — lifecycle and teardown

- `Server.Close` / ENODEV: cancel outstanding ring entries (`io_uring_enter`
  with cancel), drain CQEs, munmap, close ring fds, then run the existing
  `cancelAll`.
- Make sure `Server.loops` waitgroup also tracks the ring goroutines so
  `Wait()` semantics are preserved.

### Phase 4 — tests

- Skip uring tests when the kernel doesn't advertise `FUSE_OVER_IO_URING`
  (mirror the virtiofs skip pattern).
- Re-run `posixtest` over the uring transport.
- Add a microbenchmark for small-metadata ops (where uring should win) and
  for large sequential READ (where the splice loss shows up).

## Splice incompatibility (READ zero-copy regression)

Today's splice-based READ reply relies on /dev/fuse's `splice_write`: a
pipe-to-fd splice with the backing file's pages, which the kernel stitches
into the reply with no copy.

Uring-FUSE replies aren't writes to an fd — they're a SQE
(`COMMIT_AND_FETCH`) whose payload location is the pre-registered entry
buffer. There is no kernel hook to consume spliced pages as the reply, and:

- The entry buffer is anonymous memory, not a spliceable fd.
- `vmsplice` can't act as a zero-copy sink for file pages.
- Replying on /dev/fuse for a uring-fetched request violates the protocol;
  the kernel tracks channel ownership per unique.
- Chaining `IORING_OP_SPLICE` doesn't help — the second splice still needs
  a real fd destination.

Closest available win: `IORING_OP_READ_FIXED` from the backing file into the
registered entry buffer. Removes the `pread` syscall and uses the fixed-
buffer fast path, but the kernel still copies page cache → buffer.

A real fix would require a kernel-side opcode along the lines of "commit
with payload from this pipe / page refs". Not merged as of mid-2026.

Net effect of enabling uring:
- Wins: batching, per-CPU locality, no syscall on the hot request path.
- Losses: READ zero-copy.
- Workload guidance: small-metadata-heavy = win, large-streaming-READ =
  regression vs splice.

## Relationship to `internal/vhostuser/`

No meaningful code sharing. Both transports terminate at
`ProtocolServer.HandleRequest`, and that is the only shared surface.

`internal/vhostuser/` is a userspace virtio device: Unix-socket vhost-user
control protocol, guest-physical memory region tracking
(`deviceregion.go`), and virtio split-ring walking against the guest's
memory layout (`virtq.go`: avail/used/desc rings, event index, inflight
tracking, batched popQueue).

uring-FUSE is a host-kernel feature: the FUSE kernel module hands the host
userspace pre-registered buffers via `io_uring`. No guest, no virtqueues,
no GPA→HVA translation, no vhost-user socket. The buffer region is a flat
host mmap addressed by entry index, not a desc ring.

The overlap is conceptual ("ring of pre-mapped buffers, fetch then
commit"); the structures, syscalls, and lifecycle are disjoint. Trying to
extract a common "ring transport" abstraction would be premature — there
is no third user, and the two ABIs disagree on every concrete detail.

Right factoring: keep `internal/vhostuser/` as-is, put uring code in
`fuse/uring_linux.go` (or `internal/fuseuring/` if it grows), and let
`ProtocolServer` remain the shared seam.

## Risks and open questions

- Kernel uring-FUSE is still young; option must default off and the legacy
  path remains the supported transport.
- Per-entry payload cap in the current kernel bounds `MaxWrite` when uring
  is on; need to clamp during INIT.
- ~~CPU pinning interacts with Go's scheduler; needs measurement before
  committing to `LockOSThread`.~~ Resolved: pinning is required for
  correctness (see Phase 2).
- Notification path (still on /dev/fuse) and uring request path share the
  same `Server`; the existing `writeMu` already serializes notify writes
  and is unaffected.

## Rough size

~1500–2500 LOC, almost entirely in `fuse/uring_linux.go`, plus a small
`MountOptions` field and an INIT-flag tweak in `server.go`. No changes to
`RawFileSystem`, `nodefs`, or `request`.

## Performance findings (post-implementation)

Measured on an 8-core / 2-CPU laptop (i5-8350U) with
`BenchmarkFindSize` (serial-per-process `find -size +5` over a flat
tree, ~1 s attr cache) and `BenchmarkStatDepth` (`fio --iodepth=N`-style
in-process stat fan-out, zero attr cache). Stats below are per request.

### Where uring beats /dev/fuse

- **Per-request handler cost is lower** (14.7 µs vs 19.3 µs on
  `BenchmarkFindSize`): we skip `read(/dev/fuse)` / `writev(/dev/fuse)`
  syscalls and the kernel's fuse_dev_read/write paths. The classic
  splice fast-path narrows the gap for big READs (which uring
  *loses*, see "Splice incompatibility"), but for metadata ops uring's
  per-request work is strictly cheaper.
- **Syscall amortization** kicks in once requests are bursty enough
  that multiple CQEs land in one `io_uring_enter`. With
  `uringDefaultEntries=32` and concurrent stat load, we observe
  ~4.7 CQEs/enter (peaks above 5 on individual queues). That's the
  intended win and it materially shows up as ~40% lower ns/op on
  `BenchmarkStatDepth/depth=256`.

### Where uring loses or fails to gain

- **Sequential-per-client workloads** (one `find` process per CPU)
  see no batching: every enter wakes for a single CQE. The handler
  is faster but the wall-clock is dominated by kernel scheduling
  latency between requests. End-to-end uring can be 1.5–2× *slower*
  than classic in this regime.
- **Per-queue serial dispatch.** The queue goroutine handles each CQE
  inline before peeking the next. With handler latency ~17 µs, a queue's
  ceiling is ~59 k req/s no matter how deep the kernel-side queue is.
  Three CQEs in one enter take `3 × handler_time` before the goroutine
  blocks again. This is the dominant remaining bottleneck on
  high-throughput tests.
- **Concurrency cap = `num_possible_cpus`.** Classic spawns new reader
  goroutines on demand (HWM scales with traffic up to `MaxReaders`).
  Uring is structurally pinned at one handler goroutine per CPU; the
  kernel's `task_cpu(current)` routing forbids fewer queues than
  `num_possible_cpus`, and our dispatcher forbids more than one
  goroutine per queue.

### Memory cost — the 192-core problem

Each entry pre-allocates a payload buffer sized to negotiated
`MaxWrite`. Total resident:

    uringDefaultEntries × num_possible_cpus × (MaxWrite + ~600 B)

| CPUs | `MaxWrite` | entries | pinned RSS |
|-----:|-----------:|--------:|-----------:|
| 8    | 128 KiB    | 32      | ~32 MiB    |
| 192  | 128 KiB    | 32      | ~768 MiB   |
| 192  | 1 MiB      | 32      | ~6 GiB     |
| 192  | 4 KiB      | 32      | ~24 MiB    |

The kernel ABI does not let us reduce queue count below
`num_possible_cpus` (every qid must be registered or
`fuse_block_alloc` blocks any new request forever on
`is_ring_ready`). The only userspace knobs are:

1. **Right-size `MaxWrite` for the workload** — metadata-heavy mounts
   waste 99% of the buffer; 4 KiB per entry is plenty.
2. **Right-size `uringDefaultEntries` by CPU count** — high CPU
   count is itself parallelism, so per-queue depth can drop. A
   plausible default is `min(32, max(4, 256/num_cpus))`.
3. **Lazy registration** — register each qid with a single
   placeholder entry and grow on demand. Conflicts with the
   simple "all entries registered at startup" model.

### Why CQE/enter is bounded by `uringDefaultEntries`

Per-queue, at most `entries` requests can be parked in registered
slots. The kernel can post at most that many CQEs before it runs
out of slots. So CQE/enter ≤ entries on each queue. Going beyond
the observed 4.7 average requires both more entries *and* enough
client parallelism to keep them filled; on small machines the
client side runs out first.

### Reply timing — what the current loop guarantees

The dispatch loop drains all available CQEs, runs each handler
inline, queues the `COMMIT_AND_FETCH` SQE, then calls
`io_uring_enter` to submit the batch *and* wait for the next
batch. The kernel only sees our reply SQEs at the next `enter`
syscall. Consequence: if 3 CQEs land at once, reply 0 is delayed
by `handler_time(1) + handler_time(2)` before the kernel releases
it to the requesting task. The batching saves syscalls; it does
not save per-request latency. For latency-sensitive single-client
workloads this is the wrong trade-off, and the right fix is
per-request worker goroutines so handlers run in parallel and
reply submission decouples from handler ordering.

### Recommended next-step work (not done in this commit)

1. **Pool `inBuf` / `inTogether`.** Two per-request allocations on
   the hot path. The classic transport uses `requestAlloc` for
   exactly this — reusing it (or a uring-specific equivalent)
   should knock several µs off `handle()`.
2. **Per-queue worker pool.** Move `q.handle` off the queue
   goroutine onto a small worker pool, with a mutex-protected SQ
   for COMMIT submissions. Removes the per-queue serial-dispatch
   ceiling and decouples reply submission from request ordering.
   This is the single largest remaining throughput win for our
   target workloads.
3. **Workload-aware `MaxWrite` clamp** in `handleInit` when
   `EnableIoUring` is on: metadata-only mounts can declare
   4 KiB and save almost all the pinned-buffer RSS.
4. **`IORING_SETUP_SQPOLL`** as an experimental toggle. Removes
   the `enter`-to-submit syscall entirely, at the cost of one
   kernel polling thread per ring. Probably only attractive after
   (2) is in place, since otherwise the syscalls aren't the
   bottleneck.

### Headline numbers

| benchmark            | classic     | uring        | notes                       |
|----------------------|-------------|--------------|-----------------------------|
| FindSize, P=4, cpu=1 | 39204 ns/op | 80116 ns/op  | uring loses, no batching    |
| StatDepth, depth=256, cpu=2 | 53496 ns/op | 90261 ns/op | uring loses, only 2 active queues |
| StatDepth, depth=256, cpu=8 | 20218 ns/op | **12295 ns/op** | uring wins by ~40%, 4.70 CQE/enter |

The crossover is consistent: uring wins when (a) the workload is
bursty enough to feed CQE/enter > 2, and (b) Go has enough
concurrency to keep multiple queues active simultaneously. Below
those thresholds the per-CPU pinning + per-queue serial dispatch
overhead exceeds the syscall savings.
