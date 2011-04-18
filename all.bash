#!/bin/sh
set -eux


for d in fuse zipfs unionfs example/loopback example/zipfs \
    example/bulkstat example/multizip example/unionfs \
    example/autounionfs ; \
do
  gomake -C $d "$@"
done

for d in fuse zipfs unionfs
do
  (cd $d && gotest )
done
