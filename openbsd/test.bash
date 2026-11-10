#!/bin/bash
# Run go-fuse tests inside an OpenBSD VM under QEMU. Needs no root.
#
# OpenBSD ships no cloud images, so the first run installs OpenBSD
# from the official install ISO into a cached disk image: drive.py
# switches the boot loader to the serial console, runs an unattended
# install with auto_install.conf from the installer shell, then logs
# in to the new system to install rc.local. Later runs boot a qcow2
# overlay of that image. rc.local runs run.sh from the CD carrying the
# cross-compiled test binaries; it tars logs and exit statuses onto a
# scratch disk and powers off. The host then unpacks the scratch disk
# and reports.
#
# fuse(4) speaks the Linux FUSE protocol only since mid-2026, so the
# default is the latest snapshot.
#
# Usage: openbsd/test.bash [package ...]   (default: ./fs)
#
# fuse(4) lacks many features that the fs tests exercise (caching,
# notifications, INTERRUPT, ...), so by default only TestPosix runs.
#
# Environment:
#   OPENBSD_VERSION   release, eg. 8.0, or "snapshots" (default)
#   OPENBSD_MIRROR    mirror base URL (default https://cdn.openbsd.org/pub/OpenBSD)
#   OPENBSD_REINSTALL set to 1 to rebuild the cached installed image
#   TEST_ARGS         flags for the test binaries (default -test.run=^TestPosix$)
#   VM_TIMEOUT        wall-clock limit for the test VM (default 30m)
#   INSTALL_TIMEOUT   wall-clock limit for the install VM (default 30m)
#   OUT_DIR           where to leave logs (default: a fresh tmp dir)
#
# Exit status is 0 only if every test binary exited with 0.

set -eu -o pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE/.."

OPENBSD_VERSION=${OPENBSD_VERSION:-snapshots}
OPENBSD_MIRROR=${OPENBSD_MIRROR:-https://cdn.openbsd.org/pub/OpenBSD}
VM_TIMEOUT=${VM_TIMEOUT:-30m}
INSTALL_TIMEOUT=${INSTALL_TIMEOUT:-30m}
TEST_ARGS=${TEST_ARGS-'-test.run=^TestPosix$'}

CACHE_DIR=$HOME/.cache/go-fuse-openbsd

if [[ $# -gt 0 ]]; then
  PACKAGES=("$@")
else
  PACKAGES=(./fs)
fi

for tool in qemu-system-x86_64 qemu-img xorriso curl python3 sha256sum; do
  command -v $tool >/dev/null || { echo "missing $tool" >&2; exit 2; }
done

ACCEL=(-accel tcg)
if [[ -w /dev/kvm ]]; then
  ACCEL=(-accel kvm -cpu host)
else
  echo "/dev/kvm not usable; falling back to slow TCG emulation" >&2
fi

mkdir -p "$CACHE_DIR"
WORK=$(mktemp -d "$CACHE_DIR/run.XXXXXXXX")
OUT_DIR=${OUT_DIR:-$WORK/results}
mkdir -p "$OUT_DIR"
trap 'rm -rf "$WORK/vm.qcow2" "$WORK/results.img" "$WORK/cidata" "$WORK/tests.iso" "$WORK/install"; rmdir "$WORK" 2>/dev/null || true' EXIT

# 1. Find and fetch the install ISO. It is never written to.
SETS_URL=$OPENBSD_MIRROR/$OPENBSD_VERSION/amd64
if curl -fsSL --output "$WORK/SHA256" "$SETS_URL/SHA256"; then
  ISO_NAME=$(grep -oE 'install[0-9]+\.iso' "$WORK/SHA256" | head -1)
  [[ -n "$ISO_NAME" ]] || { echo "no install ISO listed in $SETS_URL/SHA256" >&2; exit 1; }
  ISO_SUM=$(sed -nE "s/^SHA256 \($ISO_NAME\) = ([0-9a-f]+)$/\1/p" "$WORK/SHA256")
  rm -f "$WORK/SHA256"
else
  # Offline: use the newest cached installation.
  cached=$(ls -t "$CACHE_DIR/$OPENBSD_VERSION"-install*.qcow2 2>/dev/null | head -1)
  [[ -n "$cached" ]] || { echo "cannot reach $SETS_URL and nothing cached" >&2; exit 1; }
  echo "cannot reach $SETS_URL; using $cached" >&2
  ISO_NAME=$(basename "${cached%.qcow2}.iso")
  ISO_NAME=${ISO_NAME#"$OPENBSD_VERSION"-}
  ISO_SUM=
fi
ISO=$CACHE_DIR/$OPENBSD_VERSION-$ISO_NAME
BASE_IMAGE=$CACHE_DIR/$OPENBSD_VERSION-${ISO_NAME%.iso}.qcow2

if [[ -n "$ISO_SUM" ]] && { [[ ! -f "$ISO" ]] || ! echo "$ISO_SUM  $ISO" | sha256sum -c --status; }; then
  echo "downloading $SETS_URL/$ISO_NAME"
  curl -fL --output "$ISO.part" "$SETS_URL/$ISO_NAME"
  echo "$ISO_SUM  $ISO.part" | sha256sum -c --status || { echo "checksum mismatch for $ISO_NAME" >&2; exit 1; }
  mv "$ISO.part" "$ISO"
  # A new snapshot needs a new install.
  rm -f "$BASE_IMAGE"
fi

# 2. Install OpenBSD once into $BASE_IMAGE. The ISO is used as is:
# cdbr cannot find cdboot on an ISO rewritten by xorriso.
install_openbsd() {
  local d=$WORK/install
  mkdir -p "$d"

  local qemu=(qemu-system-x86_64 "${ACCEL[@]}" -m 2G -smp 2
    -display none -serial stdio -no-reboot -nic none
    -drive file="$d/disk.qcow2",format=qcow2,if=virtio)

  qemu-img create -q -f qcow2 "$d/disk.qcow2" 20G
  echo "installing OpenBSD from $ISO_NAME; console log in $OUT_DIR/install.log"
  timeout --foreground "$INSTALL_TIMEOUT" python3 -I "$HERE/drive.py" "$OUT_DIR/install.log" \
    install "$HERE/auto_install.conf" "$d/monitor.sock" \
    "${qemu[@]}" -drive file="$ISO",media=cdrom,readonly=on -boot d || {
    echo "OpenBSD install failed; see $OUT_DIR/install.log" >&2
    exit 1
  }
  echo "configuring installed OpenBSD"
  timeout --foreground "$INSTALL_TIMEOUT" python3 -I "$HERE/drive.py" "$OUT_DIR/install.log" \
    config "$HERE/rc.local" "$d/monitor.sock" "${qemu[@]}" || {
    echo "OpenBSD configuration failed; see $OUT_DIR/install.log" >&2
    exit 1
  }
  mv "$d/disk.qcow2" "$BASE_IMAGE"
}

if [[ ! -f "$BASE_IMAGE" || "${OPENBSD_REINSTALL:-}" == 1 ]]; then
  install_openbsd
fi
qemu-img create -q -f qcow2 -F qcow2 -b "$BASE_IMAGE" "$WORK/vm.qcow2"

# 3. Build test binaries and the test CD.
CIDATA=$WORK/cidata
mkdir -p "$CIDATA/bin"
names=()
for pkg in "${PACKAGES[@]}"; do
  name=$(basename "$pkg")
  GOOS=openbsd GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$CIDATA/bin/$name.test" "$pkg"
  names+=("$name")
done

cp "$HERE/run.sh" "$CIDATA/run.sh"
echo "${names[*]}" > "$CIDATA/tests.txt"
echo "$TEST_ARGS" > "$CIDATA/args.txt"

xorriso -report_about SORRY -as mkisofs -quiet -V gofuse -J -R -o "$WORK/tests.iso" "$CIDATA"

# 4. Boot. Disk order matters: sd0 = system, sd1 = results.
truncate -s 256M "$WORK/results.img"

echo "booting OpenBSD ($ISO_NAME); console log in $OUT_DIR/console.log"
vm_status=0
qemu-system-x86_64 \
  "${ACCEL[@]}" \
  -m 2G -smp 2 \
  -display none -serial stdio -monitor none \
  -no-reboot \
  -drive file="$WORK/vm.qcow2",format=qcow2,if=virtio \
  -drive file="$WORK/results.img",format=raw,if=virtio \
  -cdrom "$WORK/tests.iso" \
  -nic none \
  < /dev/null > >(tee "$OUT_DIR/console.log") 2>&1 &
qemu_pid=$!

# Do not rely on the guest powering off: once the results are on the
# scratch disk, give it a minute, then stop it.
to_seconds() {
  case $1 in
  *h) echo $((${1%h} * 3600)) ;;
  *m) echo $((${1%m} * 60)) ;;
  *s) echo ${1%s} ;;
  *) echo $1 ;;
  esac
}
deadline=$((SECONDS + $(to_seconds "$VM_TIMEOUT")))
finished_at=
while kill -0 $qemu_pid 2>/dev/null; do
  if [[ -z "$finished_at" ]] && tar -tf "$WORK/results.img" 2>/dev/null | grep -qx './finished'; then
    finished_at=$SECONDS
  fi
  if [[ -n "$finished_at" && $SECONDS -gt $((finished_at + 60)) ]]; then
    echo "VM did not power off after writing results; stopping it" >&2
    kill $qemu_pid
  elif [[ $SECONDS -gt $deadline ]]; then
    echo "VM timed out after $VM_TIMEOUT; stopping it" >&2
    kill $qemu_pid
    vm_status=124
  fi
  sleep 2
done
wait $qemu_pid || vm_status=${vm_status/#0/$?}

# 5. Report.
tar -xf "$WORK/results.img" -C "$OUT_DIR" 2>/dev/null || true

if [[ ! -f "$OUT_DIR/finished" ]]; then
  echo "VM did not finish the test run (qemu/timeout exit $vm_status); see $OUT_DIR/console.log" >&2
  exit 1
fi

overall=0
echo
echo "OpenBSD $OPENBSD_VERSION results ($(cat "$OUT_DIR/uname.txt" 2>/dev/null)):"
for t in "${names[@]}"; do
  st=$(cat "$OUT_DIR/$t.status" 2>/dev/null || echo missing)
  if [[ "$st" == 0 ]]; then
    echo "  ok    $t"
  else
    echo "  FAIL  $t (exit $st)"
    grep -E '^\s*--- FAIL' "$OUT_DIR/$t.log" 2>/dev/null | sed 's/^/        /' || true
    overall=1
  fi
done
echo "logs in $OUT_DIR"
exit $overall
