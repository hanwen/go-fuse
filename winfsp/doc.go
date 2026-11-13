// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package winfsp serves go-fuse file systems on Windows, using WinFsp
// (https://winfsp.dev). It needs no cgo: the WinFsp DLL is loaded at
// run time, and calls back into Go. Only amd64 and arm64 are
// supported, because Go callbacks cannot take 64-bit arguments on
// 32-bit Windows. Elsewhere, Mount and MountRaw return an error that
// wraps errors.ErrUnsupported. See docs/windows.md for setup and the
// design.
//
// WinFsp calls are translated to FUSE requests, which are handled by a
// fuse.ProtocolServer. File systems written against the fs package or
// fuse.RawFileSystem work unchanged, as long as they build on Windows;
// fs.LoopbackNode does not.
//
// Semantics differ from a FUSE mount on Linux:
//
//   - Names are case-sensitive, as on Linux; some Windows programs
//     assume that they are not. Names with characters that are invalid
//     on Windows, such as ':' or '\', are not supported.
//   - All requests are made as the user that mounted the file system,
//     and files are reported as owned by that user. The permission
//     bits map to a security descriptor. Changing the owner is not
//     supported.
//   - FILE_ATTRIBUTE_READONLY on a file maps to clearing all write
//     bits; clearing it makes the file writable by the owner. On
//     directories and symlinks it is ignored, as Windows ignores it on
//     directories.
//   - A file is deleted when the last handle that asked for deletion is
//     closed, as is usual on Windows. UNLINK or RMDIR is sent then.
//   - Symlinks are reparse points; a symlink to a directory is a
//     directory symlink. Only relative targets are supported: creating
//     a symlink with an absolute target fails, and reading one fails
//     with access denied. Creating a symlink replaces the file that
//     Windows creates first, so its inode number changes.
//   - Caching works differently; see below.
//   - Entry and inode notifications (eg. fs.Inode.NotifyEntry,
//     NotifyDelete and NotifyContent) invalidate the caches, and
//     reach Windows as change notifications. The other notifications
//     return ENOSYS.
//   - Requests are never interrupted.
//   - Unmounting releases the files that are still open, and calls
//     OnUnmount.
//   - Hard links, named streams and extended attributes are not
//     supported.
//
// # Caching
//
// The WinFsp kernel driver caches file information (size, times,
// attributes), security descriptors, directory listings and volume
// information for one second. After a write, the reported times are
// those from before the write until this cache expires. Entry and
// inode notifications drop the cache early.
//
// Below that, names are cached for the entry timeout of the file
// system, as by the kernel. Attributes are not cached: when the WinFsp
// cache expires, they are fetched with GETATTR, so the attribute
// timeout of the file system has no effect.
//
// File data is not cached: every read and write reaches the file
// system. So the FOPEN flags for the kernel page cache
// (FOPEN_DIRECT_IO, FOPEN_KEEP_CACHE and FOPEN_CACHE_DIR) make no
// difference.
//
// Data caching is not enabled, because WinFsp only caches file data
// if it also caches file information forever, for the whole volume.
// Those caches are then only dropped by a notification from the file
// system, or when a file is overwritten through the mount. Unlike on
// Linux, opening a file without FOPEN_KEEP_CACHE does not drop cached
// data, and the attribute timeout does not apply. A file system whose
// files change other than through the mount, such as one backed by a
// local directory, would show stale sizes and contents indefinitely.
//
// # Licensing
//
// WinFsp is licensed under the GPLv3, with an exception for free and
// open source software. go-fuse only loads the DLL and does not
// include WinFsp, but programs that use this package may need WinFsp's
// commercial license if they are not open source.
package winfsp
