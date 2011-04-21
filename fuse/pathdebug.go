package fuse
import (
	"fmt"
)

var _ = fmt.Println

type PathFilesystemDebug struct {
	// TODO - use a generic callback system instead.
	Connector *PathFileSystemConnector

	WrappingPathFilesystem
}

func (me *PathFilesystemDebug) Open(path string, flags uint32) (fuseFile FuseFile, status Status)  {
	if path == ".debug" && me.Connector != nil {
		return NewReadOnlyFile([]byte(me.Connector.DebugString())), OK
	}
	return me.Original.Open(path, flags)
}

func (me *PathFilesystemDebug) GetAttr(path string) (*Attr, Status) {
	if path == ".debug" && me.Connector != nil {
		return &Attr{
			Mode: S_IFREG,
			Size: uint64(len(me.Connector.DebugString())),
		}, OK 
	}
	return me.Original.GetAttr(path)
}
	
