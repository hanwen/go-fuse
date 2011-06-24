# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/fuse

MANUAL_GOFILES=api.go \
	bufferpool.go \
	copy.go \
	default.go \
	direntry.go\
	files.go \
	fuse.go \
	handle.go \
	latencymap.go \
	lockingfs.go \
	loggingfs.go \
	loopback.go \
	misc.go \
	mount.go \
	mountstate.go \
	opcode.go \
	pathdebug.go \
	pathfilesystem.go \
	pathops.go \
	request.go \
	timingfs.go \
	timingrawfs.go \
	types.go\
	version.go \
	xattr.go \

GOFILES=$(MANUAL_GOFILES) version.gen.go

include $(GOROOT)/src/Make.pkg

version.gen.go: $(MANUAL_GOFILES)
	sh genversion.sh > $@
