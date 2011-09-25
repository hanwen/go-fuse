
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/unionfs
GOFILES=unionfs.go \
	dircache.go \
	timedcache.go \
	cachingfs.go \
	autounion.go \
	create.go \
	memunionfs.go

DEPS=../fuse

include $(GOROOT)/src/Make.pkg
