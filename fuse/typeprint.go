package fuse

import (
	"github.com/hanwen/go-fuse/raw"
)

func (a *Attr) String() string {
	return raw.Print((*raw.Attr)(a))
}
