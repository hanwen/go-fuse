#!/bin/bash
# Runs the tests against a kernel from build.bash. See README.md.

set -eu

usage="usage: $0 <kernel> [<pkg> ...] [-- <test flags>]"
kernel=${1:?$usage}
shift
[ -f "$kernel/bzImage" ] && kernel=$kernel/bzImage
[ -d "$kernel" ] && kernel=$kernel/arch/x86/boot/bzImage
[ -f "$kernel" ] || { echo "$kernel: not found" >&2; exit 1; }
kernel=$(realpath "$kernel")

pkgs=()
while [ $# -gt 0 ] && [ "$1" != -- ]; do
  pkgs+=("${1#./}")
  shift
done
[ $# -gt 0 ] && shift
flags=(-test.timeout=30m "$@")

repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
if [ ${#pkgs[@]} = 0 ]; then
  for d in $(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.Dir}}{{end}}' ./...); do
    [ "$d" = "$repo/virtiofs" ] && continue
    pkgs+=("${d#$repo/}")
  done
fi

cache=$HOME/.cache/go-fuse-kernel
busybox=$cache/busybox
if [ ! -x "$busybox" ]; then
  mkdir -p "$cache"
  curl -fL -o "$busybox.tmp" https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
  chmod +x "$busybox.tmp"
  mv "$busybox.tmp" "$busybox"
fi

out=${GO_FUSE_VM_OUT:-$(mktemp -d "${TMPDIR:-/tmp}/go-fuse-vm.XXXXXX")}
mkdir -p "$out"
work=$(mktemp -d "$cache/work.XXXXXX")
trap 'rm -rf "$work"' EXIT
share=$work/share
mkdir -p "$share/out"
truncate -s "${GO_FUSE_VM_DISK:-64G}" "$work/scratch.img"
mkfs.ext4 -q -F "$work/scratch.img"

quote() {
  local q=() a
  for a in "$@"; do
    q+=("'${a//\'/\'\\\'\'}'")
  done
  echo "${q[*]}"
}

for p in "${pkgs[@]}"; do
  dst=$share/pkg/$p
  mkdir -p "$dst"
  find "$p" -maxdepth 1 -type f -exec cp {} "$dst" \;
  [ -d "$p/testdata" ] && cp -a "$p/testdata" "$dst"
  CGO_ENABLED=0 go test -c -o "$dst/pkg.test" "./$p"
  if [ "$p" = benchmark ]; then
    CGO_ENABLED=0 go build -o "$dst/bulkstat.bin" ./benchmark/bulkstat
  fi
done

{
  for p in "${pkgs[@]}"; do
    # Tests find data through their source path, so recreate it.
    o=/share/out/${p//\//_}
    d=$(quote "$repo/$p")
    echo "echo '=== $p'"
    echo "mkdir -p $d && cp -a /share/pkg/$p/. $d/"
    echo "(cd $d && TMPDIR=/tmp ./pkg.test $(quote "${flags[@]}") > $o.txt 2>&1; echo \$? > $o.exit)"
    echo "echo '=== $p exit' \$(cat $o.exit)"
  done
} > "$share/run.sh"

root=$work/root
mkdir -p "$root"/{bin,etc,proc,sys,dev,tmp,var/tmp,share}
cp "$busybox" "$root/bin/busybox"
ln -s busybox "$root/bin/sh"
printf 'root:x:0:0:root:/root:/bin/sh\nnobody:x:65534:65534:nobody:/:/bin/false\n' > "$root/etc/passwd"
printf 'root:x:0:\nnobody:x:65534:\n' > "$root/etc/group"
cat > "$root/init" <<'EOF'
#!/bin/sh
/bin/busybox --install -s /bin
export PATH=/bin
mount -t proc none /proc
mount -t sysfs none /sys
mount -t devtmpfs none /dev
ln -s /proc/self/fd /dev/fd
mount -t ext4 /dev/vda /tmp
chmod 1777 /tmp
mount -t debugfs none /sys/kernel/debug
mount -t fusectl none /sys/fs/fuse/connections
mount -t 9p -o trans=virtio,version=9p2000.L,msize=512000 share /share
uname -a
sh /share/run.sh
if [ -e /sys/kernel/debug/kmemleak ]; then
  echo scan > /sys/kernel/debug/kmemleak
  sleep 6
  echo scan > /sys/kernel/debug/kmemleak
  cat /sys/kernel/debug/kmemleak
fi
echo '=== done'
reboot -f
EOF
chmod +x "$root/init"
(cd "$root" && find . | cpio -o -H newc --quiet | gzip -1) > "$work/initrd.gz"

accel=(-accel tcg -cpu max)
if [ -w /dev/kvm ]; then
  accel=(-enable-kvm -cpu host)
fi
smp=$(nproc)
[ "$smp" -gt 4 ] && smp=4

splats='(BUG|WARNING|INFO|UBSAN|KASAN): |Call Trace:|unreferenced object|Kernel panic'
fail=0

timeout "${GO_FUSE_VM_TIMEOUT:-1h}" qemu-system-x86_64 -M pc -m "${GO_FUSE_VM_MEM:-8G}" -smp "$smp" "${accel[@]}" \
  -kernel "$kernel" -initrd "$work/initrd.gz" \
  -virtfs local,path="$share",mount_tag=share,security_model=none \
  -drive file="$work/scratch.img",if=virtio,format=raw,cache=unsafe \
  -display none -serial stdio -monitor none -no-reboot \
  -append "console=ttyS0 panic=-1 ${GO_FUSE_VM_APPEND:-}" \
  < /dev/null | tee "$out/console.txt" || true
cp -a "$share/out/." "$out/"

if ! grep -q '^=== done' "$out/console.txt"; then
  echo "FAIL: guest did not finish" >&2
  fail=1
fi
if grep -Eq "$splats" "$out/console.txt"; then
  echo "FAIL: kernel splat, see $out/console.txt:" >&2
  grep -E "$splats" "$out/console.txt" | head -20 >&2
  fail=1
fi

echo
for p in "${pkgs[@]}"; do
  o=$out/${p//\//_}
  if [ ! -f "$o.exit" ]; then
    echo "FAIL $p: did not finish"
    fail=1
    continue
  fi
  grep -E '^(--- FAIL|Benchmark|FAIL|panic:)' "$o.txt" || true
  if [ "$(cat "$o.exit")" = 0 ]; then
    echo "ok   $p"
  else
    echo "FAIL $p: exit $(cat "$o.exit")"
    fail=1
  fi
done
echo "output in $out"
exit $fail
