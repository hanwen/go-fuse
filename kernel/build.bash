#!/bin/bash
# Builds a guest kernel from a Linux source tree. See README.md.

set -eu

usage="usage: $0 debug|bench <linux-src> [<name>]"
flavor=${1:?$usage}
src=$(realpath "${2:?$usage}")
name=${3:-$(git -C "$src" describe --always --dirty 2>/dev/null || make -s -C "$src" kernelversion)}
dir=$(cd "$(dirname "$0")" && pwd)
frags=("$dir/base.config" "$dir/$flavor.config")
[ -f "${frags[1]}" ] || { echo "$usage" >&2; exit 1; }

cache=$HOME/.cache/go-fuse-kernel
out=$cache/obj/$(basename "$src")-$(echo "$src" | md5sum | cut -c1-8)-$flavor
dest=$cache/$flavor-$name

mkdir -p "$out"
make -C "$src" O="$out" x86_64_defconfig
"$src/scripts/kconfig/merge_config.sh" -m -O "$out" "$out/.config" "${frags[@]}"
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
done < <(cat "${frags[@]}")
[ $missing = 0 ]

make -C "$src" O="$out" -j"$(nproc)" bzImage
mkdir -p "$dest"
cp "$out/arch/x86/boot/bzImage" "$dest/"
cp "$out/.config" "$dest/config"
echo "$dest/bzImage"
