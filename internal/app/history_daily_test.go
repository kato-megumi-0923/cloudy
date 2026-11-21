package app

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"

	"github.com/egoist/mygo/ui"
)

// The daily page offers the past recommendations, and opening them shows the
// days NetEase keeps.
func TestOpenHistoryDaily(t *testing.T) {
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1, Nickname: "me"}
	a.route = route{page: pageDaily}
	a.daily = []netease.Track{{ID: 1, Name: "今天的歌"}}
	// Already asked for: the page opens without another request.
	a.hist = historyDaily{forUser: 1, asked: true, days: []string{"2026-11-18"}, note: "近 5 次"}
	tt := ui.NewTester(a.view, 1200, 900)
	if err := tt.Click("历史日推"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(2 * time.Second)
	if a.route.page != pageHistoryDaily {
		t.Fatalf("page %v", a.route.page)
	}
	if !tt.HasText("近 5 次") || !tt.HasText("11/18 三") {
		t.Fatal("want the days and what NetEase says of them")
	}
}

// Picking another day shows its songs from what the page already has.
func TestPickHistoryDay(t *testing.T) {
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1, Nickname: "me"}
	a.route = route{page: pageHistoryDaily}
	a.hist = historyDaily{
		forUser: 1, asked: true,
		days:  []string{"2026-11-18", "2026-11-17"},
		cache: map[string][]netease.Track{"2026-11-17": {{ID: 2, Name: "第二天的歌"}}},
		date:  "2026-11-18",
		songs: []netease.Track{{ID: 1, Name: "第一天的歌"}},
	}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Advance(2 * time.Second)
	if !tt.HasText("第一天的歌") {
		t.Fatal("want the songs of the day picked")
	}
	if err := tt.Click("11/17 二"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.hist.date != "2026-11-17" || a.hist.songs[0].ID != 2 {
		t.Fatalf("picked %q with %+v", a.hist.date, a.hist.songs)
	}
	if !tt.HasText("第二天的歌") || tt.HasText("第一天的歌") {
		t.Fatal("want the songs of the day picked now")
	}
}

// Playing a day leaves out the songs that cannot play, saying how many.
func TestPlayHistorySkips(t *testing.T) {
	a := newTestApp()
	songs := []netease.Track{
		{ID: 1, Name: "plays", Privilege: &netease.Privilege{ID: 1, St: 0, Pl: 320000}},
		{ID: 2, Name: "taken down", Privilege: &netease.Privilege{ID: 2, St: -1}},
	}
	a.playHistory(songs, false)
	if got := a.pl.Queue(); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("queue %+v", got)
	}
	if a.toast != "有 1 首暂时不能播放，已跳过" {
		t.Fatalf("said %q", a.toast)
	}
}

// With NM_FRAMES set, saves the page of the past daily recommendations, in
// the light and the dark, for looking at it.
func TestHistoryDailyFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to a directory to save frames")
	}
	cover := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			cover.SetRGBA(x, y, color.RGBA{
				R: uint8(60 + 90*x/64),
				G: uint8(95 + 60*y/64),
				B: uint8(130 + 90*x/64),
				A: 255,
			})
		}
	}
	coverPath := filepath.Join(t.TempDir(), "cover.png")
	f, err := os.Create(coverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, cover); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	songs := make([]netease.Track, 0, 12)
	for i := range 12 {
		songs = append(songs, netease.Track{
			ID:        int64(i + 1),
			Name:      "歌曲" + strconv.Itoa(i+1),
			Artists:   []netease.Artist{{Name: "某位歌手"}},
			Album:     netease.Album{Name: "某张专辑", PicURL: coverPath},
			Duration:  int64(180+i*7) * 1000,
			Privilege: &netease.Privilege{ID: int64(i + 1), St: 0, Pl: 320000},
		})
	}
	songs[2].Privilege = &netease.Privilege{ID: 3, St: -1}
	for _, dark := range []bool{false, true} {
		appearance := "light"
		if dark {
			appearance = "dark"
		}
		a := newTestApp()
		a.settings = defaultSettings()
		a.profile = &netease.Profile{UserID: 1, Nickname: "me"}
		a.art.add(coverPath, &artwork{bmp: ui.NewBitmap(cover), tint: ui.RGB(70, 110, 150), used: time.Now(), size: 1})
		a.hist = historyDaily{
			forUser: 1, asked: true,
			days:  []string{"2026-11-19", "2026-11-18", "2026-11-17", "2026-11-16", "2026-11-15"},
			note:  "您已尊享查看60天内近5次历史日推的特权",
			date:  "2026-11-19",
			songs: songs,
		}
		a.open(route{page: pageHistoryDaily})
		tt := ui.NewTester(a.view, 1180, 800)
		tt.SetDark(dark)
		tt.Advance(2 * time.Second)
		tt.Frame()
		out, err := os.Create(filepath.Join(dir, "history-daily-"+appearance+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(out, tt.Image()); err != nil {
			out.Close()
			t.Fatal(err)
		}
		out.Close()
	}
}
