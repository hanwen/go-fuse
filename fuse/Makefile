# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/fuse

MANUAL_GOFILES=api.go \
	bufferpool.go \
	copy.go \
	default.go \
	defaultfile.go \
	defaultraw.go \
	direntry.go\
	files.go \
	fsmount.go \
	fsinode.go \
	fsconnector.go \
	fuse.go \
	handle.go \
	inode.go \
	latencymap.go \
	lockingfs.go \
	loggingfs.go \
	loopback.go \
	misc.go \
	mount.go \
	mountstate.go \
	opcode.go \
	pathdebug.go \
	fsops.go \
	readonlyfs.go \
	request.go \
	switchfs.go \
	timingfs.go \
	timingrawfs.go \
	types.go \
	version.go \
	xattr.go \

GOFILES=$(MANUAL_GOFILES) version.gen.go

include $(GOROOT)/src/Make.pkg

version.gen.go: $(MANUAL_GOFILES)
	sh genversion.sh > $@
