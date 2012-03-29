package raw
import "fmt"
	
func (me *ForgetIn) String() string {
	return fmt.Sprintf("{%d}", me.Nlookup)
}

func (me *BatchForgetIn) String() string {
	return fmt.Sprintf("{%d}", me.Count)
}


func (me *MkdirIn) String() string {
	return fmt.Sprintf("{0%o (0%o)}", me.Mode, me.Umask)
}

func (me *MknodIn) String() string {
	return fmt.Sprintf("{0%o (0%o), %d}", me.Mode, me.Umask, me.Rdev)
}
