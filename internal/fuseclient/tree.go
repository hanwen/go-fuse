// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package fuseclient

import (
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
)

// Node is a FUSE node that the Tree knows about.
type Node struct {
	ID uint64

	// protected by Tree.mu
	mode    uint32 // file type bits
	lookups uint64
	// refs counts the references from Walk, Lookup, Add and
	// Ref. A node with references is not forgotten.
	refs     int
	children map[string]*dentry
	// parent and name are the last link that we resolved; used
	// to compute a path for notifications.
	parent *Node
	name   string
	links  int
}

type dentry struct {
	node   *Node
	expiry time.Time
}

// Tree resolves slash-separated paths to nodes, caching entries for
// their entry timeout. It keeps lookup counts, and sends FORGET when a
// node is no longer reachable and not open.
type Tree struct {
	*Client

	mu    sync.Mutex
	nodes map[uint64]*Node
	root  *Node

	now func() time.Time
}

// NewTree returns a Tree for the root of c.
func NewTree(c *Client) *Tree {
	root := &Node{ID: fuse.FUSE_ROOT_ID, links: 1}
	return &Tree{
		Client: c,
		nodes:  map[uint64]*Node{root.ID: root},
		root:   root,
		now:    time.Now,
	}
}

// Root returns the root node.
func (t *Tree) Root() *Node { return t.root }

// Split splits a path into components, ignoring empty ones.
func Split(path string) []string {
	var res []string
	for _, c := range strings.Split(path, "/") {
		if c != "" {
			res = append(res, c)
		}
	}
	return res
}

// Walk resolves a path. It returns the attributes of the last
// component (from LOOKUP or GETATTR). If a component other than the
// last does not exist, it returns ENOTDIR or ENOENT with a nil node and
// missingParent set. The returned node holds a temporary reference that
// must be dropped with Unref.
//
// Walk does not follow symlinks. Empty components are ignored, "."
// and ".." fail with EINVAL, and a trailing slash fails with ENOTDIR
// unless the path is a directory.
func (t *Tree) Walk(caller *fuse.Caller, path string) (n *Node, attr fuse.Attr, missingParent bool, errno syscall.Errno) {
	// held are the nodes from lookup; their references are dropped
	// on return, so a directory stays known while we look up its
	// children.
	var held []*Node
	defer func() {
		for _, n := range held {
			t.Unref(n)
		}
	}()

	n = t.root
	comps := Split(path)
	if len(comps) == 0 {
		out, errno := t.Getattr(caller, n.ID, nil)
		if errno != 0 {
			return nil, fuse.Attr{}, false, errno
		}
		attr = out.Attr
	}
	for i, c := range comps {
		last := i == len(comps)-1
		child, e, errno := t.lookup(caller, n, c, last)
		if errno != 0 {
			return nil, fuse.Attr{}, !last, errno
		}
		held = append(held, child)
		if !last && e.Attr.Mode&S_IFMT != fuse.S_IFDIR {
			return nil, fuse.Attr{}, true, syscall.ENOTDIR
		}
		n = child
		attr = e.Attr
	}
	if strings.HasSuffix(path, "/") && attr.Mode&S_IFMT != fuse.S_IFDIR {
		return nil, fuse.Attr{}, false, syscall.ENOTDIR
	}
	// The deferred function drops the reference from lookup.
	t.Ref(n)
	return n, attr, false, 0
}

// maxSymlinks is the number of symlinks that WalkFollow resolves
// before failing with ELOOP, as MAXSYMLINKS in Linux.
const maxSymlinks = 40

// WalkFollow resolves a path like Walk, but follows symlinks in all
// components, including the last, and handles "." and "..".
// Symlink targets are relative to the directory holding the link;
// absolute targets and paths that leave the root fail with EXDEV. A
// trailing slash, also in a symlink target, requires a directory. The
// returned node holds a temporary reference that must be dropped with
// Unref.
func (t *Tree) WalkFollow(caller *fuse.Caller, path string) (*Node, fuse.Attr, syscall.Errno) {
	// stack holds the nodes leading to the current one; each
	// entry except the root holds a reference.
	stack := []*Node{t.root}
	defer func() {
		for _, n := range stack[1:] {
			t.Unref(n)
		}
	}()

	todo := splitDir(path)
	links := 0
	var attr fuse.Attr
	haveAttr := false
	for len(todo) > 0 {
		c := todo[0]
		todo = todo[1:]
		switch c {
		case ".":
			continue
		case "..":
			if len(stack) == 1 {
				return nil, fuse.Attr{}, syscall.EXDEV
			}
			t.Unref(stack[len(stack)-1])
			stack = stack[:len(stack)-1]
			haveAttr = false
			continue
		}

		n, e, errno := t.lookup(caller, stack[len(stack)-1], c, true)
		if errno != 0 {
			return nil, fuse.Attr{}, errno
		}
		stack = append(stack, n)
		switch e.Attr.Mode & S_IFMT {
		case fuse.S_IFLNK:
			links++
			if links > maxSymlinks {
				return nil, fuse.Attr{}, syscall.ELOOP
			}
			target, errno := t.Readlink(caller, n.ID)
			if errno != 0 {
				return nil, fuse.Attr{}, errno
			}
			if strings.HasPrefix(target, "/") {
				return nil, fuse.Attr{}, syscall.EXDEV
			}
			stack = stack[:len(stack)-1]
			t.Unref(n)
			todo = append(splitDir(target), todo...)
			continue
		case fuse.S_IFDIR:
		default:
			if len(todo) > 0 {
				return nil, fuse.Attr{}, syscall.ENOTDIR
			}
		}
		attr = e.Attr
		haveAttr = true
	}

	n := stack[len(stack)-1]
	if !haveAttr {
		out, errno := t.Getattr(caller, n.ID, nil)
		if errno != 0 {
			return nil, fuse.Attr{}, errno
		}
		attr = out.Attr
	}
	// The deferred function drops the stack's reference.
	t.Ref(n)
	return n, attr, 0
}

// splitDir splits a path like Split, but keeps a trailing slash as a
// final "." component.
func splitDir(path string) []string {
	comps := Split(path)
	if strings.HasSuffix(path, "/") {
		comps = append(comps, ".")
	}
	return comps
}

// Lookup returns the child `name` of dir, from cache or by sending
// LOOKUP. The returned node holds a temporary reference that must be
// dropped with Unref. If the entry came from the cache, the
// returned EntryOut has a fresh Attr from GETATTR.
func (t *Tree) Lookup(caller *fuse.Caller, dir *Node, name string) (*Node, fuse.EntryOut, syscall.Errno) {
	return t.lookup(caller, dir, name, true)
}

// cachedChild returns the child name of dir and its file type if
// the entry is cached and not expired. The node holds a temporary
// reference.
func (t *Tree) cachedChild(dir *Node, name string) (*Node, uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	d := dir.children[name]
	if d == nil || !t.now().Before(d.expiry) {
		return nil, 0
	}
	d.node.refs++
	return d.node, d.node.mode
}

func (t *Tree) lookup(caller *fuse.Caller, dir *Node, name string, needAttr bool) (*Node, fuse.EntryOut, syscall.Errno) {
	if name == "." || name == ".." {
		// The file system resolves these to other nodes,
		// which the cache must not record as children.
		return nil, fuse.EntryOut{}, syscall.EINVAL
	}
	if n, mode := t.cachedChild(dir, name); n != nil {
		if !needAttr {
			return n, fuse.EntryOut{NodeId: n.ID, Attr: fuse.Attr{Mode: mode}}, 0
		}
		out, errno := t.Getattr(caller, n.ID, nil)
		if errno != 0 {
			t.Unref(n)
			if errno == syscall.ENOENT {
				t.Invalidate(dir, name)
			}
			return nil, fuse.EntryOut{}, errno
		}
		return n, fuse.EntryOut{NodeId: n.ID, Attr: out.Attr}, 0
	}

	out, errno := t.Client.Lookup(caller, dir.ID, name)
	if errno != 0 {
		if errno == syscall.ENOENT {
			t.Invalidate(dir, name)
		}
		return nil, out, errno
	}
	n, errno := t.Add(dir, name, &out)
	if errno != 0 {
		return nil, fuse.EntryOut{}, errno
	}
	return n, out, 0
}

// Add records an entry returned by LOOKUP, CREATE, MKDIR, SYMLINK or
// READDIRPLUS, which counts as one lookup. It returns the node with a
// temporary reference.
//
// Like the kernel's d_splice_alias, a directory has at most one
// entry: if it is cached under another name, that entry moves. If
// the directory is dir or one of its ancestors, Add forgets the
// lookup and fails with ELOOP.
func (t *Tree) Add(dir *Node, name string, out *fuse.EntryOut) (*Node, syscall.Errno) {
	n, forgets, errno := t.add(dir, name, out)
	t.sendForgets(forgets)
	return n, errno
}

func (t *Tree) add(dir *Node, name string, out *fuse.EntryOut) (*Node, []forget, syscall.Errno) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := t.nodes[out.NodeId]
	if n == nil {
		n = &Node{ID: out.NodeId}
		t.nodes[n.ID] = n
	}
	expiry := t.now().Add(out.EntryTimeout())
	if d := dir.children[name]; d != nil && d.node == n {
		n.lookups++
		n.refs++
		n.mode = out.Attr.Mode & S_IFMT
		d.expiry = expiry
		return n, nil, 0
	}

	isDir := out.Attr.Mode&S_IFMT == fuse.S_IFDIR
	if isDir && n.links > 0 {
		for a := dir; a != nil; a = a.parent {
			if a == n {
				return nil, []forget{{n.ID, 1}}, syscall.ELOOP
			}
		}
	}

	n.lookups++
	n.refs++
	n.mode = out.Attr.Mode & S_IFMT
	var forgets []forget
	if isDir && n.links > 0 && n.parent != nil {
		// Move the directory's only entry.
		if d := n.parent.children[n.name]; d != nil && d.node == n {
			delete(n.parent.children, n.name)
			n.links--
		}
	}
	forgets = t.unlinkLocked(dir, name)
	if dir.children == nil {
		dir.children = map[string]*dentry{}
	}
	dir.children[name] = &dentry{node: n, expiry: expiry}
	n.links++
	n.parent = dir
	n.name = name
	return n, forgets, 0
}

// Ref adds a reference to n, eg. for an open file.
func (t *Tree) Ref(n *Node) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n.refs++
}

// Unref drops a reference from Lookup, Add or Ref.
func (t *Tree) Unref(n *Node) {
	t.mu.Lock()
	n.refs--
	forgets := t.maybeForgetLocked(n, nil)
	t.mu.Unlock()
	t.sendForgets(forgets)
}

// Invalidate drops the cached entry for name in dir, if any.
func (t *Tree) Invalidate(dir *Node, name string) {
	t.mu.Lock()
	forgets := t.unlinkLocked(dir, name)
	t.mu.Unlock()
	t.sendForgets(forgets)
}

// InvalidateID drops the cached entry for name in the directory
// with the given ID, if it is known. This is for
// fuse.Notify.InvalEntry.
func (t *Tree) InvalidateID(dirID uint64, name string) {
	t.mu.Lock()
	var forgets []forget
	if dir := t.nodes[dirID]; dir != nil {
		forgets = t.unlinkLocked(dir, name)
	}
	t.mu.Unlock()
	t.sendForgets(forgets)
}

// Move updates the cache after a successful RENAME.
func (t *Tree) Move(dir *Node, name string, newDir *Node, newName string) {
	t.mu.Lock()
	var forgets []forget
	d := dir.children[name]
	if d != nil {
		delete(dir.children, name)
	}
	forgets = append(forgets, t.unlinkLocked(newDir, newName)...)
	if d != nil {
		if newDir.children == nil {
			newDir.children = map[string]*dentry{}
		}
		newDir.children[newName] = d
		d.node.parent = newDir
		d.node.name = newName
	}
	t.mu.Unlock()
	t.sendForgets(forgets)
}

// Path returns a path for the node with the given ID, if known.
func (t *Tree) Path(id uint64) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := t.nodes[id]
	if n == nil {
		return "", false
	}
	var comps []string
	for n != t.root {
		if n.parent == nil || n.links == 0 {
			return "", false
		}
		comps = append(comps, n.name)
		n = n.parent
	}
	for i, j := 0, len(comps)-1; i < j; i, j = i+1, j-1 {
		comps[i], comps[j] = comps[j], comps[i]
	}
	return "/" + strings.Join(comps, "/"), true
}

// Lookups returns the outstanding lookup count for each node, for
// tests.
func (t *Tree) Lookups() map[uint64]uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	res := map[uint64]uint64{}
	for id, n := range t.nodes {
		if n != t.root {
			res[id] = n.lookups
		}
	}
	return res
}

// ForgetAll drops all cached entries, sending FORGET for nodes that
// are not referenced.
func (t *Tree) ForgetAll() {
	t.mu.Lock()
	var forgets []forget
	for name := range t.root.children {
		forgets = append(forgets, t.unlinkLocked(t.root, name)...)
	}
	t.mu.Unlock()
	t.sendForgets(forgets)
}

// Close releases the handles that are still open, forgets all nodes,
// including referenced ones, and sends DESTROY, as the kernel does
// when the file system is unmounted. The Tree must not be used
// afterwards.
func (t *Tree) Close() {
	t.releaseAll()
	t.mu.Lock()
	var forgets []forget
	for id, n := range t.nodes {
		if n != t.root && n.lookups > 0 {
			forgets = append(forgets, forget{id, n.lookups})
			n.lookups = 0
		}
	}
	t.nodes = map[uint64]*Node{t.root.ID: t.root}
	t.root.children = nil
	t.mu.Unlock()
	t.sendForgets(forgets)
	t.Client.Close()
}

type forget struct {
	id, n uint64
}

func (t *Tree) unlinkLocked(dir *Node, name string) []forget {
	d := dir.children[name]
	if d == nil {
		return nil
	}
	delete(dir.children, name)
	d.node.links--
	return t.maybeForgetLocked(d.node, nil)
}

// maybeForgetLocked forgets n if it is unreachable. Unreachable
// directories also drop their children.
func (t *Tree) maybeForgetLocked(n *Node, acc []forget) []forget {
	if n == t.root || n.links > 0 {
		return acc
	}
	for name, d := range n.children {
		delete(n.children, name)
		d.node.links--
		acc = t.maybeForgetLocked(d.node, acc)
	}
	if n.refs > 0 || n.lookups == 0 {
		return acc
	}
	acc = append(acc, forget{n.ID, n.lookups})
	n.lookups = 0
	if t.nodes[n.ID] == n {
		delete(t.nodes, n.ID)
	}
	return acc
}

func (t *Tree) sendForgets(fs []forget) {
	for _, f := range fs {
		t.Forget(f.id, f.n)
	}
}

// S_IFMT is the file type mask in the FUSE protocol (Linux values).
const S_IFMT = 0170000
