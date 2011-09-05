package fuse
import (
	"log"
)

var _ = log.Println

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes, ie. FSes that
// disallow hardlinks.
type fsInode struct {
	*inode
	Name   string

	// This is nil at the root of the mount.
	Parent *fsInode
}

// GetPath returns the path relative to the mount governing this
// inode.  It returns nil for mount if the file was deleted or the
// filesystem unmounted.  This will take the treeLock for the mount,
// so it can not be used in internal methods.
func (me *fsInode) GetPath() (path string, mount *fileSystemMount) {
	me.inode.treeLock.RLock()
	defer me.inode.treeLock.RUnlock()

	if me.inode.mount == nil {
		// Node from unmounted file system.
		return ".deleted", nil
	}

	rev_components := make([]string, 0, 10)
	n := me
	for ; n.Parent != nil; n = n.Parent {
		rev_components = append(rev_components, n.Name)
	}
	if n.mountPoint == nil {
		return ".deleted", nil
	}
	return ReverseJoin(rev_components, "/"), n.mountPoint
}

func (me *fsInode) addChild(name string, ch *fsInode) {
	if ch.inode.mountPoint == nil {
		ch.Parent = me
	}
	ch.Name = name
}

func (me *fsInode) rmChild(name string, ch *fsInode) {
	ch.Name = ".deleted"
	ch.Parent = nil
}
