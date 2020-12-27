// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestRmChildParallel(t *testing.T) {
	want := "hello"
	root := &Inode{}
	_, _, clean := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()

			var wg sync.WaitGroup
			var nms []string
			for i := 0; i < 100; i++ {
				nms = append(nms, fmt.Sprint(i))
			}
			for _, nm := range nms {
				wg.Add(1)
				go func(nm string) {
					ch := n.NewPersistentInode(
						ctx,
						&MemRegularFile{
							Data: []byte(want),
							Attr: fuse.Attr{
								Mode: 0464,
							},
						},
						StableAttr{})
					n.AddChild(nm, ch, false)
					wg.Done()
				}(nm)
			}
			for _, nm := range nms {
				wg.Add(1)
				go func(nm string) {
					n.RmChild(nm)
					wg.Done()
				}(nm)
			}

			wg.Wait()
		},
	})
	defer clean()
}
