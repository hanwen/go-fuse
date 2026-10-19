#!/bin/bash
# Builds a guest kernel for the virtiofs tests from a Linux source tree.
#
#   build.bash <linux-src> [<build-dir>]
#
# Then run the tests with GO_FUSE_VIRTIOFS_KERNEL=<build-dir>/arch/x86/boot/bzImage.

set -eu

src=${1:?usage: $0 <linux-src> [<build-dir>]}
out=${2:-$HOME/.cache/go-fuse-virtiofs/linux}
frag=$(cd "$(dirname "$0")" && pwd)/virtiofs.config

mkdir -p "$out"
make -C "$src" O="$out" x86_64_defconfig
"$src/scripts/kconfig/merge_config.sh" -m -O "$out" "$out/.config" "$frag"
make -C "$src" O="$out" olddefconfig

missing=0
while read -r line; do
  case "$line" in
    ''|'#'*) continue ;;
    *=n) want="# ${line%=n} is not set" ;;
    *) want=$line ;;
  esac
  if ! grep -qxF "$want" "$out/.config"; then
    echo "config: want '$want'" >&2
    missing=1
  fi
done < "$frag"
[ $missing = 0 ]

make -C "$src" O="$out" -j"$(nproc)" bzImage
echo "$out/arch/x86/boot/bzImage"
