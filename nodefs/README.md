
Objective
=========

A high-performance FUSE API that minimizes pitfalls with writing
correct filesystems.

Decisions
=========

   * Nodes contain references to their children. This is useful
     because most filesystems will need to construct tree-like
     structures.

   * Nodes can be "persistent", meaning their lifetime is not under
     control of the kernel. This is useful for constructing FS trees
     in advance, rather than driven by LOOKUP.

   * The NodeID for FS tree node must be defined on creation and are
     immutable. By contrast, reusing NodeIds (eg. rsc/bazil FUSE, as
     well as old go-fuse/fuse/nodefs) needs extra synchronization to
     avoid races with notify and FORGET, and makes handling the inode
     Generation more complicated.
     
   * The mode of an Inode is defined on creation.  Files cannot change
     type during their lifetime. This also prevents the common error
     of forgetting to return the filetype in Lookup/GetAttr.
     
   * The NodeID (used for communicating with kernel) is equal to
     Attr.Ino (value shown in Stat and Lstat return values.). 

   * No global treelock, to ensure scalability.

   * Support for hard links. libfuse doesn't support this in the
     high-level API.  Extra care for race conditions is needed when
     looking up the same file through different paths.

   * do not issue Notify{Entry,Delete} as part of
     AddChild/RmChild/MvChild: because NodeIDs are unique and
     immutable, there is no confusion about which nodes are
     invalidated, and the notification doesn't have to happen under
     lock.

   * Directory reading uses the DirStream. Semantics for rewinding
     directory reads, and adding files after opening (but before
     reading) are handled automatically. No support for directory
     seeks.


To decide
=========

   * Should we provide automatic fileID numbering?
   
   * One giant interface with many methods, or many one-method
     interfaces? Or some interface (file, dir, symlink, etc).
 
   * one SetAttr method, or many (Chown, Truncate, etc.)

   * function signatures, or types? The latter is easier to remember?
     Easier to extend? The latter less efficient (indirections/copies)

```
    func Lookup(name string, out *EntryOut) (Node, Status) {
    }

or

    type LookupIn {
       Name string
    }
    type LookupOut {
       fuse.EntryOut
    }

    func Lookup(ctx context.Context, in *LookupIn, out *LookupOut) 
```

   * What to do with semi-unused fields (CreateIn.Umask, OpenIn.Mode, etc.)
   
   * Readlink return: []byte or string ?

   * Should Operations.Lookup return *Inode or Operations ?

   * Should bridge.Lookup() add the child, bridge.Unlink remove the child, etc.?

   * Merge Fsync/FsyncDir?

   * Merge Release/ReleaseDir?
 
