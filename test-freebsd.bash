#!/bin/bash

set -eux

CACHE_DIR=$HOME/.cache/go-fuse-freebsd/
TMP=$(mktemp -d $CACHE_DIR/tmp.XXXXXXXXX)
FREEBSD_VERSION=${FREEBSD_VERSION:-15.0}
IMAGE_NAME=FreeBSD-${FREEBSD_VERSION}-RELEASE-amd64-ufs.raw
FREEBSD_IMAGE_URL=${FREEBSD_IMAGE_URL:-https://download.freebsd.org/releases/VM-IMAGES/${FREEBSD_VERSION}-RELEASE/amd64/Latest/${IMAGE_NAME}.xz}
IMAGE_RAW=$CACHE_DIR/$IMAGE_NAME

# 1. Download and convert to raw, once. Pristine base, never modified.
mkdir -p "$CACHE_DIR"
if [[ ! -f "$IMAGE_RAW" ]]; then
  curl -fL --output "$IMAGE_RAW.xz" "$FREEBSD_IMAGE_URL"
  xz -d "$IMAGE_RAW.xz"
fi

cp $CACHE_DIR/$IMAGE_NAME $TMP/image.raw

DEV=$(sudo losetup --show --find -P $TMP/image.raw)
mkdir -p $TMP/mnt
UFS=$HOME/bin/fuse-ufs-bin
sudo $UFS -o rw ${DEV}p4 $TMP/mnt
GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go test -c -o $TMP ./posixtest
GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go test -c -o $TMP ./fs

cat << EOF | sudo tee -a $TMP/mnt/boot/loader.conf 
autoboot_delay="0"
console="comconsole"
EOF

sudo cp $TMP/posixtest.test $TMP/fs.test $TMP/mnt/
cat << EOF | sudo tee  $TMP/mnt/etc/rc.local
#!/bin/sh

kldload fusefs

/posixtest.test -posixdir /tmp > /posixtest.log  2>&1
/fs.test -test.v > /fs.log 2>&1
sync
shutdown -p now

EOF

sudo umount $TMP/mnt

qemu-system-x86_64 \
  -enable-kvm -cpu host \
  -m 2G -smp 2 -nographic \
  -drive file=$TMP/image.raw,format=raw,if=virtio \
  -netdev user,id=net0,hostfwd=tcp::2222-:22 \
  -device virtio-net-pci,netdev=net0 \
  -nographic

sudo $UFS -o rw ${DEV}p4 $TMP/mnt
sudo cp $TMP/mnt/posixtest.log $TMP/posixtest.log
sudo cp $TMP/mnt/fs.log $TMP/
sudo umount $TMP/mnt

