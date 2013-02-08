package raw
import (
	"syscall"
)

func init() {
	OpenFlagNames[syscall.O_DIRECT] = "DIRECT"
	OpenFlagNames[syscall.O_LARGEFILE] = "LARGEFILE"
	OpenFlagNames[syscall_O_NOATIME] = "NOATIME"
}
