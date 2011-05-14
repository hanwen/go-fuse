# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/fuse

GOFILES=api.go \
	misc.go\
	request.go \
	fuse.go\
	direntry.go\
	mount.go\
	types.go\
	pathfilesystem.go \
	bufferpool.go \
	default.go \
	files.go \
	lockingfs.go \
	loopback.go \
	timingfs.go \
	timingrawfs.go \
	xattr.go \
	pathdebug.go \
	opcode.go \
	pathops.go \
	latencymap.go \
	loggingfs.go \
	version.gen.go

version.gen.go:
	echo "package fuse" > $@
	git log -n1 --pretty=format:'const version = "%h (%cd)"' --date=iso >> $@

include $(GOROOT)/src/Make.pkg

