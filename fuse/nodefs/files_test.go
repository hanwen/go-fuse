package nodefs

import (
	"testing"
	"time"
)

func TestTimeToTimeval(t *testing.T) {
	// Check that dates before 1970 are handled correctly
	date := time.Date(1960, time.January, 1, 23, 16, 44, 650951, time.UTC)
	tv := timeToTimeval(&date)
	if tv.Sec != -315535396 || tv.Usec != 650 {
		t.Fail()
	}

	// Check recent date
	date = time.Date(2015, time.September, 29, 20, 8, 7, 74522, time.UTC)
	tv = timeToTimeval(&date)
	if tv.Sec != 1443557287 || tv.Usec != 74 {
		t.Fail()
	}
}
