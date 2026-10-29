//go:build amd64 || arm64

package barrier

func Write()
func Read()
func Full()
func LoadUint16(ptr *uint16) uint16
