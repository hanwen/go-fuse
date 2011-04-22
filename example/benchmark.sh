#!/bin/sh

# Runtime is typically dominated by the costs of GetAttr (ie. Stat),
# so let's time that.  We use zipfs which runs from memory to minimize
# noise due to the filesystem itself.

set -eux

ZIPFILE=$1
shift

DELAY=5

gomake -C zipfs
gomake -C bulkstat

MP=/tmp/zipbench
fusermount -u ${MP} || true
mkdir -p ${MP}

ZIPFS=$PWD/zipfs/zipfs
BULKSTAT=$PWD/bulkstat/bulkstat

cd /tmp

${ZIPFS} ${MP} ${ZIPFILE} >& zipfs.log &


# Wait for FS to mount.
sleep ${DELAY}
find ${MP} > /tmp/zipfiles.txt
fusermount -u ${MP}

# The command below should be profiled.
${ZIPFS} ${MP} ${ZIPFILE} >& zipfs.log &

# Wait for zipfs to unpack and serve the file.
sleep ${DELAY}

# Warm caches.
${BULKSTAT} -runs 1 /tmp/zipfiles.txt

# Performance number without 6prof running
${BULKSTAT} -runs 5 /tmp/zipfiles.txt

echo -e "\n\n"

# Run 6prof
6prof -p $! -d 20 -t 3 -hs -l -h -f >& /tmp/zipfs.6prof &
sleep 0.1

# Feed data to 6prof
${BULKSTAT} -runs 3 /tmp/zipfiles.txt

echo -e "\n\n"

fusermount -u /tmp/zipbench

cat zipfs.log
