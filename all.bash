#!/bin/sh
set -eux

for target in "clean" "install" ; do
  for d in fuse fuse/pathfs fuse/test zipfs unionfs \
    example/hello example/loopback example/zipfs \
    example/multizip example/unionfs example/memfs \
    example/autounionfs ; \
  do
    if test "${target}" = "install" && test "${d}" = "fuse/test"; then
      continue
    fi
    go ${target} github.com/hanwen/go-fuse/${d}
  done
done

for d in fuse zipfs unionfs
do
  (cd $d && go test github.com/hanwen/go-fuse/$d && go test -race github.com/hanwen/go-fuse/$d)
done

make -C benchmark
for d in benchmark
do
  go test github.com/hanwen/go-fuse/benchmark -test.bench '.*' -test.cpu 1,2
done
