# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/zipfs
DEPS=../fuse

GOFILES=zipfs.go \
	multizip.go \
	tarfs.go \
	memtree.go

include $(GOROOT)/src/Make.pkg

