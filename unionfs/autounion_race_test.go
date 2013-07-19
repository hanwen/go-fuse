// +build !race

package unionfs

import (
	"os"
	"testing"
	"time"
)

// Ideally, this should check inject a logger rather than a boolean,
// so we can check if the messages are as we expect.  Right now, this
// test requires visual inspection. When run with -v, the UNLINK reply
// and SYMLINK request are not printed.
func TestToggleDebug(t *testing.T) {
	wd, clean := setup(t)
	defer clean()

	os.Remove(wd + "/mnt/status/debug_setting")
	time.Sleep(10 * time.Millisecond)

	if err := os.Symlink("xyz", wd+"/mnt/status/debug_setting"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// now we should be in debug mode.
	link, err := os.Readlink(wd + "/mnt/status/debug_setting")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if link != "1" {
		t.Errorf("got %q want %q for debug_setting readlink",
			link, "1")
	}
}
