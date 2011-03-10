# Use "gomake install" to build and install this package.
include $(GOROOT)/src/Make.inc

all: loopback multizip

# Start c&p make.cmd 
%: %._.$O
	$(LD) -o $@ _go_.$O

%._.$O: %.go $(PREREQ)
	$(GC) -o $@ $< 
# end c&p

DEPS=../examplelib

include $(GOROOT)/src/Make.cmd

