package fuse

import (
	"testing"
)

func TestSwitchFsSlash(t *testing.T) {
	fsMap := []SwitchedFileSystem{
		SwitchedFileSystem{Prefix: ""},
		SwitchedFileSystem{Prefix: "/home"},
		SwitchedFileSystem{Prefix: "usr/"},
	}

	sfs := NewSwitchFileSystem(fsMap)
	for path, expectPrefix := range map[string]string{
		"home/foo/bar": "home",
		"usr/local":    "usr",
	} {
		_, fs := sfs.findFileSystem(path)
		if fs.Prefix != expectPrefix {
			t.Errorf("Mismatch %s - '%s' != '%s'", path, fs.Prefix, expectPrefix)
		}
	}
}

func TestSwitchFs(t *testing.T) {
	fsMap := []SwitchedFileSystem{
		SwitchedFileSystem{Prefix: ""},
		SwitchedFileSystem{Prefix: "home/foo"},
		SwitchedFileSystem{Prefix: "home"},
		SwitchedFileSystem{Prefix: "usr"},
	}

	sfs := NewSwitchFileSystem(fsMap)

	for path, expectPrefix := range map[string]string{
		"xyz":           "",
		"home/foo/bar":  "home/foo",
		"home/fooz/bar": "home",
		"home/efg":      "home",
		"lib":           "",
		"abc":           "",
		"usr/local":     "usr",
	} {
		_, fs := sfs.findFileSystem(path)
		if fs.Prefix != expectPrefix {
			t.Errorf("Mismatch %s %s %v", path, fs.Prefix, expectPrefix)
		}
	}
}

func TestSwitchFsStrip(t *testing.T) {
	fsMap := []SwitchedFileSystem{
		SwitchedFileSystem{Prefix: ""},
		SwitchedFileSystem{Prefix: "dev", StripPrefix: true},
		SwitchedFileSystem{Prefix: "home", StripPrefix: false},
	}

	sfs := NewSwitchFileSystem(fsMap)
	// Don't check for inputs ending in '/' since Go-FUSE never
	// generates them.
	for path, expectPath := range map[string]string{
		"xyz":          "xyz",
		"home/foo/bar": "home/foo/bar",
		"home":         "home",
		"dev/null":     "null",
		"dev":          "",
	} {
		stripPath, _ := sfs.findFileSystem(path)
		if stripPath != expectPath {
			t.Errorf("Mismatch %s %s %v", path, stripPath, expectPath)
		}
	}
}

func TestSwitchFsApi(t *testing.T) {
	var fs FileSystem
	fs = &SwitchedFileSystem{}
	_ = fs
}
