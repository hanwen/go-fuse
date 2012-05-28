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

	dst, code := destFs.Create(destFile, uint32(os.O_WRONLY|os.O_CREATE|os.O_TRUNC), attr.Mode, context)
	if !code.Ok() {
		return code
	}
	defer dst.Release()
	defer dst.Flush()

	buf := make([]byte, 128*(1<<10))
	off := int64(0)
	for {
		res, code := src.Read(buf, off)
		if !code.Ok() {
			return code
		}
		data := res.Bytes(buf)

		if len(data) == 0 {
			break
		}
		n, code := dst.Write(data, off)
		if !code.Ok() {
			return code
		}
		if int(n) < len(data) {
			return EIO
		}
		if len(data) < len(buf) {
			break
		}
		off += int64(len(data))
	}
	return OK
}
