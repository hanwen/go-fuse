#!/bin/sh

# Benchmark to test speedup for caching in the r/o layer.

export GOMAXPROCS=$(grep ^processor /proc/cpuinfo|wc -l)

set -eux

fusermount -u /tmp/zipunion || true
fusermount -u /tmp/zipbench || true

gomake -C example/unionfs
gomake -C example/zipfs

mkdir -p /tmp/zipbench
./example/zipfs/zipfs /tmp/zipbench /usr/lib/jvm/java-1.6.0-openjdk-1.6.0.0/src.zip &
sleep 1

mkdir -p /tmp/ziprw /tmp/zipunion
./example/unionfs/unionfs /tmp/zipunion /tmp/ziprw /tmp/zipbench &
sleep 1

wc /tmp/zipunion/javax/lang/model/element/UnknownAnnotationValueException.java

echo hello >> /tmp/zipunion/javax/lang/model/element/UnknownAnnotationValueException.java

# Heat caches.
time ls -lR /tmp/zipbench/ > /dev/null
sleep 1s
time ls -lR /tmp/zipunion/ > /dev/null
sleep 5s
time ls -lR  /tmp/zipunion/ > /dev/null

