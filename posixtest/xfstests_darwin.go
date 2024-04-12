package posixtest

func (d *dirent) off() int64 {
	return int64(d.Seekoff)
}

func (d *dirent) ino() uint64 {
	return d.Ino
}
