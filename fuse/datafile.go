package fuse

// A FUSE file for read-only filesystems.  This assumes we already have the data in memory.

type ReadOnlyFile struct {
	data []byte

	DefaultRawFuseFile
}

func NewReadOnlyFile(data []byte) *ReadOnlyFile {
	f := new(ReadOnlyFile)
	f.data = data
	return f
}

func (self *ReadOnlyFile) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	end := int(input.Offset) + int(input.Size)
	if end > len(self.data) {
		end = len(self.data)
	}

	return self.data[input.Offset:end], OK
}
