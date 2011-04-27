# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/fuse

GOFILES=misc.go\
	fuse.go\
	direntry.go\
	mount.go\
	types.go\
	pathfilesystem.go \
	bufferpool.go \
	default.go \
	datafile.go \
	lockingfs.go \
	loopback.go \
	timingfs.go \
	timingrawfs.go \
	xattr.go \
	devnull.go \
	pathdebug.go \
	opcode.go \
	pathops.go \
	latencymap.go

include $(GOROOT)/src/Make.pkg

