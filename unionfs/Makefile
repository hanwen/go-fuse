
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/unionfs
GOFILES=unionfs.go \
	dircache.go \
	timedcache.go \
	cachingfs.go \
	autounion.go \
	create.go

DEPS=../fuse ../zipfs

include $(GOROOT)/src/Make.pkg
