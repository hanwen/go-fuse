
Objective
=========

A high-performance FUSE API that minimizes pitfalls with writing
correct filesystems.

Decisions
=========

   * Nodes contain references to their children. This is useful
     because most filesystems will need to construct tree-like structures.

   * Nodes can be "persistent", meaning their lifetime is not under
     control of the kernel. This is useful for constructing FS trees
     in advance.

   * The NodeID (used for communicating with kernel) is equal to
     Attr.Ino (value shown in Stat and Lstat return values.) 

   * No global treelock, to ensure scalability.

   * Immutable characteristics of the Inode are passed on
     creation. These are {NodeID, Mode}. Files cannot change type
     during their lifetime. It also prevents the common error of
     forgetting to return the filetype in Lookup/GetAttr.

To decide
=========

   * Should we provide automatic fileID numbering?
   
   * Should OpenDir/ReadDir read the entire directory in one go?

   * One giant interface with many methods, or many one-method interfaces?
 
   * one SetAttr method, or many (Chown, Truncate, etc.)

   * function signatures, or types? The latter is easier to remember?
     Easier to extend?

```
    func Lookup(name string, out *EntryOut) (Node, Status) {
    }


    type LookupOp struct {
      // in
      Name string

      // out
      Child Node
      Out *EntryOut
    }
    func Lookup(op LookupOp)
```

   * What to do with semi-unused fields (CreateIn.Umask, OpenIn.Mode, etc.)
   
   * cancellation through context.Context (standard, more GC overhead)
     or a custom context (could reuse across requests.)?

   * Readlink return: []byte or string ?

   * Should Operations.Lookup return *Inode or Operations ?

   * Should bridge.Lookup() add the child, bridge.Unlink remove the child, etc.?
