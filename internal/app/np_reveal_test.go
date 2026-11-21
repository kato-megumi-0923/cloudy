package app

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The reveal rounds the composed view without stretching its contents at fractional display scaling.
func TestNowPlayingRevealKeepsCornersAndContentPosition(t *testing.T) {
	r := ui.Rect{X: 19.5, Y: 17.5, W: 82, H: 66}
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.RGB(255, 255, 255))
		ui.Box(c).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).Clip().Children(func() {
			ui.Box(c).Absolute().Left(-r.X).Top(-r.Y).Size(160, 100).
				Effect(nowPlayingReveal{rect: r, radius: 15}).Children(func() {
				ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).Width(74).Background(ui.RGB(0, 0, 255))
				ui.Box(c).Absolute().Top(0).Bottom(0).Left(74).Right(0).Background(ui.RGB(0, 255, 0))
			})
		})
	}, 160, 100)
	const scale = 1.5
	tt.SetScale(scale)
	img := tt.Image()
	for _, check := range []struct {
		x, y float32
		want color.RGBA
	}{
		{20, 18, color.RGBA{255, 255, 255, 255}},
		{38, 23, color.RGBA{0, 0, 255, 255}},
		{72, 42, color.RGBA{0, 0, 255, 255}},
		{77, 42, color.RGBA{0, 255, 0, 255}},
		{104, 42, color.RGBA{255, 255, 255, 255}},
	} {
		if got := img.RGBAAt(int(check.x*scale), int(check.y*scale)); got != check.want {
			t.Errorf("pixel at (%g, %g) = %v, want %v", check.x, check.y, got, check.want)
		}
	}
}
