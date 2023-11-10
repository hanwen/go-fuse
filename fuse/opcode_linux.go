package fuse

func explicitDataCacheMode(inputFlags uint32) uint32 {
	return inputFlags & CAP_EXPLICIT_INVAL_DATA
}
