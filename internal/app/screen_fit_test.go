package app

import (
	"testing"

	"github.com/egoist/mygo"
)

// A primary 2560x1600 at 150% (1706x1066 DIPs) and a 1920x1080 at 100% to
// its right, as #29 had them; spots off both come back, those on one are
// kept within it.
func TestFitInKeepsWindowsOnScreens(t *testing.T) {
	primary := mygo.Rectangle{X: 0, Y: 0, Width: 1706, Height: 1018}
	second := mygo.Rectangle{X: 2560, Y: 0, Width: 1920, Height: 1032}
	both := []mygo.Rectangle{primary, second}
	for _, c := range []struct {
		name       string
		areas      []mygo.Rectangle
		x, y       int
		wantX      int
		wantY      int
		wantOnline bool
	}{
		{"on the primary", both, 100, 200, 100, 200, true},
		{"on the second", both, 3000, 500, 3000, 500, true},
		{"past the second's edge", both, 4400, 500, 4480 - 420, 500, true},
		{"scaled off both", both, 6000, 500, 6000, 500, false},
		{"second unplugged", []mygo.Rectangle{primary}, 3000, 500, 3000, 500, false},
		{"half off the top", both, 100, -50, 100, 0, true},
	} {
		x, y, ok := fitIn(c.areas, c.x, c.y, 420, 116)
		if ok != c.wantOnline || (ok && (x != c.wantX || y != c.wantY)) {
			t.Errorf("%s: got (%d, %d) %v, want (%d, %d) %v", c.name, x, y, ok, c.wantX, c.wantY, c.wantOnline)
		}
	}
}
