package printer

import (
	"epos-proxy/internal/testutil"
	"github.com/google/gousb"
	"testing"
)

func TestPathToString(t *testing.T) {
	desc := &gousb.DeviceDesc{
		Bus:  1,
		Path: []int{2, 3, 4},
	}

	got := pathToString(desc)
	testutil.ExpectedEqual(t, got, "1.2.3.4")
}
