#!/bin/bash
set -eux

# Kernel version is relevant to debugging CI failures
uname -a

# Everything must compile on Linux
go build ./...

# Not everything compiles on MacOS (try GOOS=darwin go build ./...).
# But our key packages should.
GOOS=darwin go build ./fs/... ./example/loopback/... ./fskit/...
GOOS=freebsd go build ./fs/... ./example/loopback/... 

GOARCH=386 GOOS=linux CGO_ENABLED=0 go build ./fs/... ./example/loopback/...
GOARCH=arm GOOS=linux CGO_ENABLED=0 go build ./fs/... ./example/loopback/...

# Run the tests. Why the flags:
# -timeout 5m ... Get a backtrace on a hung test before the CI system kills us
# -p 1 .......... Run tests serially, which also means we get live output
#                 instead of per-package buffering.
# -count 1 ...... Disable result caching, so we can see flakey tests
GO_TEST="go test -timeout 5m -p 1 -count 1"
# Run all tests as current user
$GO_TEST ./...

# 32-bit userspace against the 64-bit kernel. Needs CONFIG_COMPAT, and on
# arm64 a CPU that can run AArch32 (many cannot), so probe first.
case $(go env GOARCH) in
amd64) GOARCH32=386 ;;
arm64) GOARCH32=arm ;;
*) GOARCH32= ;;
esac
if [[ -n "$GOARCH32" ]]; then
  probe=$(mktemp)
  if GOARCH=$GOARCH32 CGO_ENABLED=0 go test -c -o $probe ./splice && $probe -test.run '^$'; then
    GOARCH=$GOARCH32 CGO_ENABLED=0 $GO_TEST ./fuse/... ./fs/... ./posixtest ./splice ./zipfs ./newunionfs
  else
    echo "cannot run GOARCH=$GOARCH32 binaries; skipping 32-bit tests"
  fi
  rm -f $probe
fi
# The following tests need to run as root
sudo env PATH=$PATH $GO_TEST -run 'Test(DirectMount|Forget|Passthrough|IDMappedMount|ReconnectByFileHandle|SetXAttrKillSgid)' ./fs ./fuse

# NFS export reconnect test: needs root (for name_to_handle_at-adjacent
# privileges and exportfs) plus rpcbind/rpc.mountd/nfsd
sudo env PATH=$PATH $GO_TEST -run TestNFSExportReconnect ./example/nfs

# Run virtiofs tests (including posixtest inside a VM) if QEMU and KVM are available.
# These are skipped automatically by TestMain when assets cannot be prepared.
$GO_TEST -timeout 2m -run 'Test(Basic|Posixtest)' ./virtiofs

make -C benchmark
go test ./benchmark -test.bench '.*' -test.cpu 1,2
