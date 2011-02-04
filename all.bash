#!/bin/sh
set -eux
for d in fuse examplelib example zipfs
do
  gomake -C $d
done

for d in fuse examplelib
do
  (cd $d && gotest )
done
