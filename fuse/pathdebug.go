package fuse
import (
	"fmt"
)

var _ = fmt.Println

type FileSystemDebug struct {
	// TODO - use a generic callback system instead.
	Connector *FileSystemConnector

	WrappingFileSystem
}

func (me *FileSystemDebug) Open(path string, flags uint32) (fuseFile File, status Status)  {
	if path == ".debug" && me.Connector != nil {
		return NewReadOnlyFile([]byte(me.Connector.DebugString())), OK
	}
	return me.Original.Open(path, flags)
}

func (me *FileSystemDebug) GetAttr(path string) (*Attr, Status) {
	if path == ".debug" && me.Connector != nil {
		return &Attr{
			Mode: S_IFREG,
			Size: uint64(len(me.Connector.DebugString())),
		}, OK 
	}
	return me.Original.GetAttr(path)
}
	
