# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

all: loopback multizip

# Start c&p make.cmd
%: %._.$O
	$(LD) -o $@ $<

%._.$O: %.go $(PREREQ)
	$(GC) -o $@ $<
# end c&p

DEPS=../examplelib ../fuse

include $(GOROOT)/src/Make.cmd

