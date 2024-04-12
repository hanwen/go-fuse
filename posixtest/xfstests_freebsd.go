package posixtest

func (d *dirent) off() int64 {
	return d.Off
}

func (d *dirent) ino() uint64 {
	return d.Fileno
}
