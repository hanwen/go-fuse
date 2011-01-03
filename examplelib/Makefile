# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/examplelib

DEPS=../fuse

GOFILES=dummyfuse.go\
	passthrough.go\
	stackfs.go

include $(GOROOT)/src/Make.pkg

