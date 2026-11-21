package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// A wide window at its least height keeps the progress in it as Now
// Playing calms and its cover grows (the user, 1.3.3-test.1: it went
// under the window's foot).
func TestNowPlayingCalmKeepsProgressInWideWindow(t *testing.T) {
	for _, size := range [][2]int{{1400, 580}, {2400, 640}, {1180, 580}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			w, h := size[0], size[1]
			a := newTestApp()
			a.settings = defaultSettings()
			a.pl.Cue([]netease.Track{{ID: 1, Name: "Copines", Duration: 200000, Artists: []netease.Artist{{Name: "Aya Nakamura"}}, Album: netease.Album{Name: "NAKAMURA (Deluxe Edition)"}}})
			var lines []lyrics.Line
			for i := 0; i < 20; i++ {
				lines = append(lines, lyrics.Line{At: time.Duration(i) * 4 * time.Second, Text: fmt.Sprintf("第 %d 句歌词在这里", i)})
			}
			a.pl.SetLyrics(lines)
			a.nowPlaying = true
			old := npCursor
			npCursor = func() mygo.Point { return mygo.Point{X: 10, Y: 10} }
			defer func() { npCursor = old }()
			tt := ui.NewTester(a.view, w, h)
			tt.Move(float32(w)/2, float32(h)/2)
			tt.Frame()
			check := func(when string) {
				for _, name := range []string{"播放进度", "Copines"} {
					r, ok := tt.Find(name)
					if !ok {
						continue
					}
					if r.Y+r.H > float32(h)-8 || r.Y < 0 {
						t.Fatalf("%s: %s at %+v, out of the %d-high window", when, name, r, h)
					}
				}
			}
			for i := 0; i < 40; i++ {
				tt.Advance(32 * time.Millisecond)
				tt.Frame()
			}
			check("awake")
			for i := 0; i < int(npCalmStill/(32*time.Millisecond))+80; i++ {
				tt.Advance(32 * time.Millisecond)
				tt.Frame()
				if i > int(npCalmStill/(32*time.Millisecond)) {
					check(fmt.Sprintf("calming, frame %d", i))
				}
			}
		})
	}
}
