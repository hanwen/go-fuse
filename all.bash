#!/bin/sh
set -eux

sh genversion.sh fuse/version.gen.go

for target in "clean" "install" ; do
  for d in fuse benchmark zipfs unionfs \
    example/hello example/loopback example/zipfs \
    example/bulkstat example/multizip example/unionfs \
    example/autounionfs ; \
  do
    go ${target} go-fuse/${d}
  done
done

for d in fuse zipfs unionfs
do
  (cd $d && go test go-fuse/$d )
done

# TODO - reinstate the benchmark
exit 1

gomake -C benchmark cstatfs
for d in benchmark
do
  (cd $d && gotest -test.bench '.*' -test.cpu 1,2 )
done
