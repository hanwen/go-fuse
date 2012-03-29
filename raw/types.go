package raw


type ForgetIn struct {
	Nlookup uint64
}

type ForgetOne struct {
	NodeId  uint64
	Nlookup uint64
}

type BatchForgetIn struct {
	Count uint32
	Dummy uint32
}


type MkdirIn struct {
	Mode  uint32
	Umask uint32
}

type RenameIn struct {
	Newdir uint64
}

type LinkIn struct {
	Oldnodeid uint64
}
	
type MknodIn struct {
	Mode    uint32
	Rdev    uint32
	Umask   uint32
	Padding uint32
}

