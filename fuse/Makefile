# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/fuse
#TARG=fuse

GOFILES=misc.go\
	fuse.go\
	direntry.go\
	mount.go\
	types.go\
	pathfilesystem.go \
	bufferpool.go \
	default.go \
	datafile.go \
	loopback.go \
	wrappedfs.go \
	timingfs.go \
	timingrawfs.go

include $(GOROOT)/src/Make.pkg

