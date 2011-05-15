# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/fuse

GOFILES=\
	api.go \
	bufferpool.go \
	copy.go \
	default.go \
	direntry.go\
	files.go \
	fuse.go\
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
	version.gen.go \
	xattr.go \

include $(GOROOT)/src/Make.pkg

version.gen.go:
	echo "package fuse" > $@
	git log -n1 --pretty=format:'const version = "%h (%cd)"' --date=iso >> $@


