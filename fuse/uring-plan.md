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
