package fuse

import (
	"os"
)

func CopyFile(srcFs, destFs FileSystem, srcFile, destFile string, context *Context) Status {
	src, code := srcFs.Open(srcFile, uint32(os.O_RDONLY), context)
	if !code.Ok() {
		return code
	}
	defer src.Release()
	defer src.Flush()

	attr, code := srcFs.GetAttr(srcFile, context)
	if !code.Ok() {
		return code
	}

	w := WriteIn{
		Flags: uint32(os.O_WRONLY | os.O_CREATE | os.O_TRUNC),
	}
	dst, code := destFs.Create(destFile, w.Flags, attr.Mode, context)
	if !code.Ok() {
		return code
	}
	defer dst.Release()
	defer dst.Flush()

	buf := make([]byte, 128 * (1 << 10))
	off := int64(0)
	for {
		res := src.Read(buf, off)
		if !res.Ok() {
			return res.Status
		}
		res.Read(buf)
		
		if len(res.Data) == 0 {
			break
		}
		n, code := dst.Write(res.Data, off)
		if !code.Ok() {
			return code
		}
		if int(n) < len(res.Data) {
			return EIO
		}
		if len(res.Data) < len(buf) {
			break
		}
		off += int64(len(res.Data))
	}
	return OK
}
