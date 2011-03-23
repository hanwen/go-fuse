#!/bin/sh

# Runtime is typically dominated by the costs of GetAttr (ie. Stat),
# so let's time that.  We use zipfs which runs from memory to minimize
# noise due to the filesystem itself.

set -eux

ZIPFILE=$1
shift

gomake -C zipfs
gomake -C bulkstat

MP=/tmp/zipbench
mkdir -p ${MP}

ZIPFS=$PWD/zipfs/zipfs
BULKSTAT=$PWD/bulkstat/bulkstat

cd /tmp

${ZIPFS} ${MP} ${ZIPFILE} >& zipfs.log &
find ${MP} > /tmp/zipfiles.txt
fusermount -u /tmp/zipbench

# The command below should be profiled.
${ZIPFS} ${MP} ${ZIPFILE} >& zipfs.log &

# Wait for zipfs to unpack and serve the file.
sleep 1

# Warm caches.
${BULKSTAT} /tmp/zipfiles.txt

# C++ binaries can do this ~0.1ms/stat.
${BULKSTAT} /tmp/zipfiles.txt


fusermount -u /tmp/zipbench


cat zipfs.log
