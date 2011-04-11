#!/bin/sh
set -eux

for d in fuse examplelib example/loopback example/zipfs \
    example/bulkstat example/multizip
do
  gomake -C $d "$@"
done

for d in fuse examplelib
do
  (cd $d && gotest )
done
