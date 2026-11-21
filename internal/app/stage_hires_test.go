package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
)

// The lyrics setting applies and persists both changes without restarting.
func TestStageHiResTogglePersists(t *testing.T) {
	stage := newTestApp()
	stage.settings = loadSettingsFrom(filepath.Join(t.TempDir(), "settings.json"))
	stage.settingsView.open, stage.settingsView.tab = true, settingsLyrics
	tester := ui.NewTester(stage.view, 1000, 800)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester.Frame()
	title, found := tester.Find("高清动态文字")
	pane, paneFound := tester.Find("歌词设置项")
	if !found || !paneFound {
		t.Fatal("the high-resolution switch is not in the lyrics settings")
	}
	for _, want := range []bool{true, false} {
		tester.ClickAt(pane.X+pane.W-30-14-22, title.Y+title.H/2)
		tester.Frame()
		if got := stage.settings.StageHiRes; got != want {
			t.Fatalf("toggling high-resolution text set %t, want %t", got, want)
		}
		if got := loadSettingsFrom(stage.settings.path).StageHiRes; got != want {
			t.Fatalf("reloading high-resolution text gave %t, want %t", got, want)
		}
	}
}

// Switching the ink must not move the next word or change the row's height as a word enters and drifts.
func TestStageHiResKeepsWordLayout(t *testing.T) {
	stage := &app{settings: settings{StageSparks: true}}
	word := lyrics.Word{Text: "风过 AV ", Start: time.Second, End: 2 * time.Second}
	position := word.Start
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Padding(20).AlignItems(ui.Center).Children(func() {
			stage.stageWord(c.Key("word"), word, wordShape{scale: 1.3, rotate: 5, drift: -8, margin: 24}, position, 64, white)
			ui.Text(c.Key("neighbor"), "回声").FontSize(64)
		})
	}, 500, 180)
	tester.SetScale(1.25)
	for _, at := range []time.Duration{word.Start, word.Start + 234*time.Millisecond, word.End + 1500*time.Millisecond} {
		position = at
		stage.settings.StageHiRes = false
		tester.Frame()
		baseline, found := tester.Find("回声")
		stage.settings.StageHiRes = true
		tester.Frame()
		candidate, stillFound := tester.Find("回声")
		if !found || !stillFound || candidate != baseline {
			t.Fatalf("neighbor at %s: original %v, high-resolution %v", at, baseline, candidate)
		}
	}
}
