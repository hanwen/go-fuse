#!/bin/sh
set -eux

rm -f fuse/version.gen.go


for target in "clean" "" "$@" ; do
  for d in fuse benchmark zipfs unionfs \
    example/hello example/loopback example/zipfs \
    example/bulkstat example/multizip example/unionfs \
    example/autounionfs ; \
  do
    gomake -C $d $target
  done
done

for d in fuse zipfs unionfs
do
  (cd $d && gotest )
done

gomake -C benchmark cstatfs
for d in benchmark
do
  (cd $d && gotest -test.bench '.*' -test.cpu 1,2 )
done 
