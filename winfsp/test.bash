#!/bin/bash
# Run go-fuse tests inside a Windows VM under QEMU, with WinFsp. Needs
# no root, but needs KVM and about 10G of disk.
#
# The first run installs Windows unattended into a base image (this
# takes 20-40 minutes). By default it installs the Windows Server 2025
# evaluation, which needs no product key and no TPM. QEMU reads the ISO
# straight from Microsoft's download server, so it is not stored
# locally. Each test run boots a throwaway overlay of the base image,
# copies the cross-compiled test binaries in over SSH, runs them, and
# powers off.
#
# Usage:
#   winfsp/test.bash [package ...]      run tests (default: ./winfsp)
#   winfsp/test.bash install            only build the base image
#   winfsp/test.bash shell              boot the VM and leave it running;
#                                       prints the ssh command to use
#
# Environment:
#   WINDOWS_ISO       install ISO: a local path or https URL (default: the
#                     Server 2025 evaluation)
#   WINDOWS_IMAGE     index of the image in install.wim (default 1,
#                     Server Core Standard for the evaluation ISO)
#   WINDOWS_MSDM      ACPI MSDM table with an OEM product key, passed to
#                     the VM so a matching client edition activates.
#                     Read it from /sys/firmware/acpi/tables/MSDM as root.
#                     Not useful with the evaluation ISO.
#   WINDOWS_PRODUCT_KEY  product key for the unattended install, eg. the
#                     OEM key from `strings MSDM`. Must match the edition
#                     of WINDOWS_IMAGE; leave unset for the evaluation.
#   WINFSP_MSI_URL    WinFsp installer to use
#   TEST_ARGS         extra flags for the test binaries
#   VM_DIR            where images are kept (default ~/.cache/go-fuse-windows)
#   VM_DISPLAY        QEMU -display value (default none); eg. gtk, vnc=:5
#   VM_TIMEOUT        wall-clock limit for a test run (default 30m)
#   INSTALL_TIMEOUT   wall-clock limit for the install (default 2h)
#   OUT_DIR           where to leave logs (default: a fresh dir in VM_DIR)
#
# While the VM runs, "screendump FILE.ppm" on the monitor socket
# ($VM_DIR/monitor.sock, eg. with socat) takes a screenshot.

set -eu -o pipefail
cd "$(dirname "$0")/.."

WINDOWS_ISO=${WINDOWS_ISO:-https://software-static.download.prss.microsoft.com/dbazure/888969d5-f34g-4e03-ac9d-1f9786c66749/26100.1742.240906-0331.ge_release_svc_refresh_SERVER_EVAL_x64FRE_en-us.iso}
WINDOWS_IMAGE=${WINDOWS_IMAGE:-1}
WINFSP_MSI_URL=${WINFSP_MSI_URL:-https://github.com/winfsp/winfsp/releases/download/v2.1/winfsp-2.1.25156.msi}
VM_DIR=${VM_DIR:-$HOME/.cache/go-fuse-windows}
VM_DISPLAY=${VM_DISPLAY:-none}
VM_TIMEOUT=${VM_TIMEOUT:-30m}
INSTALL_TIMEOUT=${INSTALL_TIMEOUT:-2h}
TEST_ARGS=${TEST_ARGS:-}
PASSWORD='GoFuse-Test1!'

for tool in qemu-system-x86_64 qemu-img curl ssh scp ssh-keygen; do
  command -v $tool >/dev/null || { echo "missing $tool" >&2; exit 2; }
done
MKISO=
for tool in xorriso genisoimage mkisofs; do
  if command -v $tool >/dev/null; then MKISO=$tool; break; fi
done
[[ -n "$MKISO" ]] || { echo "need xorriso, genisoimage or mkisofs" >&2; exit 2; }
[[ -w /dev/kvm ]] || { echo "/dev/kvm is not usable; Windows is too slow without KVM" >&2; exit 2; }

mkdir -p "$VM_DIR"
BASE=$VM_DIR/base.qcow2
KEY=$VM_DIR/id_ed25519
MONITOR=$VM_DIR/monitor.sock
[[ -f "$KEY" ]] || ssh-keygen -q -t ed25519 -N '' -C go-fuse-windows -f "$KEY"

if [[ -z "${WINDOWS_MSDM:-}" && -r /sys/firmware/acpi/tables/MSDM ]]; then
  WINDOWS_MSDM=/sys/firmware/acpi/tables/MSDM
fi
MSDM_ARGS=()
if [[ -n "${WINDOWS_MSDM:-}" ]]; then
  MSDM_ARGS=(-acpitable "file=$WINDOWS_MSDM")
fi

mkiso() { # label dir out
  case $MKISO in
  xorriso) xorriso -report_about SORRY -as mkisofs -quiet -V "$1" -J -R -o "$3" "$2" ;;
  *) $MKISO -quiet -V "$1" -J -R -o "$3" "$2" ;;
  esac
}

# qemu runs the VM. Extra arguments are appended.
qemu() {
  local timeout=$1
  shift
  rm -f "$MONITOR"
  timeout --foreground "$timeout" qemu-system-x86_64 \
    -machine q35 -accel kvm -cpu host \
    -m 4G -smp 4 \
    -display "$VM_DISPLAY" -vga std \
    -monitor "unix:$MONITOR,server,nowait" \
    -rtc base=utc \
    "${MSDM_ARGS[@]}" \
    "$@"
}

install_base() {
  local work=$VM_DIR/install
  rm -rf "$work"
  mkdir -p "$work/setup"
  local msi=$VM_DIR/$(basename "$WINFSP_MSI_URL")
  if [[ ! -f "$msi" ]]; then
    echo "downloading $WINFSP_MSI_URL"
    curl -fL --output "$msi.part" "$WINFSP_MSI_URL"
    mv "$msi.part" "$msi"
  fi
  cp "$msi" winfsp/vm/setup.ps1 "$work/setup/"
  cp "$KEY.pub" "$work/setup/authorized_keys"
  local pk=
  if [[ -n "${WINDOWS_PRODUCT_KEY:-}" ]]; then
    pk="<ProductKey><Key>$WINDOWS_PRODUCT_KEY</Key></ProductKey>"
  fi
  sed -e "s/@@IMAGE_INDEX@@/$WINDOWS_IMAGE/" \
      -e "s/@@PASSWORD@@/$PASSWORD/g" \
      -e "s|@@PRODUCT_KEY@@|$pk|" \
      winfsp/vm/autounattend.xml > "$work/setup/autounattend.xml"
  mkiso GOFUSESETUP "$work/setup" "$work/setup.iso"

  local iso
  case $WINDOWS_ISO in
  https://*|http://*)
    iso="file.driver=${WINDOWS_ISO%%:*},file.url=$WINDOWS_ISO,file.readahead=4194304,file.timeout=60" ;;
  *) iso="file=$WINDOWS_ISO" ;;
  esac

  qemu-img create -q -f qcow2 "$work/disk.qcow2" 64G
  echo "installing Windows into $work/disk.qcow2; this takes a while"
  echo "  screenshots: echo 'screendump /tmp/s.ppm' | socat - UNIX-CONNECT:$MONITOR"
  qemu "$INSTALL_TIMEOUT" \
    -boot order=dc \
    -drive file="$work/disk.qcow2",format=qcow2,if=ide,index=0 \
    -drive "$iso",format=raw,media=cdrom,readonly=on,if=ide,index=1 \
    -drive file="$work/setup.iso",format=raw,media=cdrom,readonly=on,if=ide,index=2 \
    -nic none

  # setup.ps1 powers off when done; check that it got there.
  echo "checking install"
  if ! boot_and_wait "$work/disk.qcow2" ssh_ok 'Test-Path C:\gofuse\setup-done' ; then
    echo "install did not complete; image left in $work" >&2
    exit 1
  fi
  mv "$work/disk.qcow2" "$BASE"
  rm -rf "$work"
  echo "base image ready: $BASE ($(du -h "$BASE" | cut -f1))"
}

SSH_PORT=
pick_port() {
  SSH_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
}

SSH_OPTS=()
vm_ssh() {
  ssh -F none -i "$KEY" -p "$SSH_PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR -o ConnectTimeout=10 "${SSH_OPTS[@]}" Administrator@127.0.0.1 "$@"
}
vm_scp() {
  scp -F none -q -i "$KEY" -P "$SSH_PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR "$@"
}

QEMU_PID=
start_vm() { # disk
  pick_port
  qemu "$VM_TIMEOUT" \
    -drive file="$1",format=qcow2,if=ide,index=0 \
    -nic "user,model=e1000e,restrict=on,hostfwd=tcp:127.0.0.1:$SSH_PORT-:22" \
    </dev/null >"$VM_DIR/qemu.log" 2>&1 &
  QEMU_PID=$!
  local deadline=$((SECONDS + 600))
  until vm_ssh 'exit 0' >/dev/null 2>&1; do
    if ! kill -0 $QEMU_PID 2>/dev/null; then
      echo "qemu exited; see $VM_DIR/qemu.log" >&2
      return 1
    fi
    if ((SECONDS > deadline)); then
      echo "no ssh after 10 minutes" >&2
      return 1
    fi
    sleep 5
  done
}

stop_vm() {
  [[ -n "$QEMU_PID" ]] || return 0
  vm_ssh 'Stop-Computer -Force' >/dev/null 2>&1 || true
  for _ in $(seq 60); do
    kill -0 $QEMU_PID 2>/dev/null || break
    sleep 2
  done
  kill $QEMU_PID 2>/dev/null || true
  wait $QEMU_PID 2>/dev/null || true
  QEMU_PID=
}

ssh_ok() { [[ "$(vm_ssh "$1" 2>/dev/null | tr -d '\r')" == True ]]; }

boot_and_wait() { # disk check-func arg
  if ! start_vm "$1"; then
    stop_vm
    return 1
  fi
  local ok=0
  "$2" "$3" || ok=1
  stop_vm
  return $ok
}

cmd=${1:-}
if [[ "$cmd" == install ]]; then
  install_base
  exit 0
fi
[[ -f "$BASE" ]] || install_base

WORK=$(mktemp -d "$VM_DIR/run.XXXXXXXX")
OUT_DIR=${OUT_DIR:-$WORK/results}
mkdir -p "$OUT_DIR"
qemu-img create -q -f qcow2 -F qcow2 -b "$BASE" "$WORK/vm.qcow2"
trap 'stop_vm; rm -f "$WORK/vm.qcow2"; rm -rf "$WORK/bin"' EXIT

if [[ "$cmd" == shell ]]; then
  VM_TIMEOUT=24h
  start_vm "$WORK/vm.qcow2"
  echo "VM running (qemu pid $QEMU_PID). Connect with:"
  echo "  ssh -F none -i $KEY -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null Administrator@127.0.0.1"
  echo "Press Enter to shut down."
  read -r
  exit 0
fi

if [[ $# -gt 0 ]]; then
  PACKAGES=("$@")
else
  PACKAGES=(./winfsp)
fi

mkdir -p "$WORK/bin"
names=()
for pkg in "${PACKAGES[@]}"; do
  name=$(basename "$pkg")
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$WORK/bin/$name.test.exe" "$pkg"
  names+=("$name")
done

echo "booting Windows"
start_vm "$WORK/vm.qcow2"
vm_ssh 'New-Item -ItemType Directory -Force C:\gofuse\bin | Out-Null'
vm_scp "$WORK"/bin/*.test.exe "Administrator@127.0.0.1:C:/gofuse/bin/"
vm_ssh '[Environment]::OSVersion.VersionString; (Get-Item "C:\Program Files (x86)\WinFsp\bin\winfsp-x64.dll").VersionInfo.ProductVersion' \
  > "$OUT_DIR/version.txt" 2>&1 || true

overall=0
for t in "${names[@]}"; do
  echo "running $t"
  st=0
  # PowerShell 5 splits unquoted -test.v at the dot, so quote each argument.
  args="'-test.v' '-test.timeout=10m'"
  for a in $TEST_ARGS; do args+=" '$a'"; done
  vm_ssh "cd C:\\gofuse\\bin; & .\\$t.test.exe $args; exit \$LASTEXITCODE" \
    > "$OUT_DIR/$t.log" 2>&1 || st=$?
  echo $st > "$OUT_DIR/$t.status"
done
stop_vm

echo
echo "Windows results ($(tr -d '\r' < "$OUT_DIR/version.txt" | paste -sd' '))"
for t in "${names[@]}"; do
  st=$(cat "$OUT_DIR/$t.status")
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
