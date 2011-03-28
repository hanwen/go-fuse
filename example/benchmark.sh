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
${BULKSTAT} /tmp/zipfiles.txt

6prof -p $! -d 20 -t 3 -hs -l -h -f >& /tmp/zipfs.6prof &
sleep 0.1

# C++ binaries can do this ~0.2ms/stat.
echo -e "\n\n"
${BULKSTAT} /tmp/zipfiles.txt
echo -e "\n\n"


fusermount -u /tmp/zipbench

cat zipfs.log
