# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

TARG=github.com/hanwen/go-fuse/benchmark

GOFILES=benchmark.go

include $(GOROOT)/src/Make.pkg

cstatfs: statfs.cc
	g++ -O2 -Wall -std=c++0x $< `pkg-config fuse --cflags --libs` -o $@
