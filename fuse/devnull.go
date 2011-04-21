package fuse

// A FUSE file that accepts any write, and always returns EOF.
type DevNullFile struct {
	DefaultFuseFile
}

func NewDevNullFile() *DevNullFile {
	return new(DevNullFile)
}

func (me *DevNullFile) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	return []byte{}, OK
}

func (me *DevNullFile) Write(input *WriteIn, content []byte) (uint32, Status) {
	return uint32(len(content)), OK
}

func (me *DevNullFile) Flush() Status {
	return OK
}

func (me *DevNullFile) Fsync(*FsyncIn) (code Status) {
	return OK
}
