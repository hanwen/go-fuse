package fuse

func explicitDataCacheMode(_ uint32) uint32 {
	// on darwin, CAP_EXPLICIT_INVAL_DATA is not defined
	return 0
}
