package app

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// One wheel notch over the volume changes it by one percent, not a quarter.
func TestVolumeWheelStepsOnePercent(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.volume = 0.5
	if !a.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 100}) {
		t.Fatal("the wheel was not taken")
	}
	if math.Abs(a.volume-0.49) > 1e-9 {
		t.Fatalf("volume %v, want 0.49", a.volume)
	}
	a.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: -100})
	if math.Abs(a.volume-0.50) > 1e-9 {
		t.Fatalf("volume %v, want 0.50", a.volume)
	}
}

// The volume is kept in the settings as it moves, not only as the app
// quits: Windows ending the session skips the quit.
func TestVolumeKeptAsItMoves(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.volume = 0.8
	a.settings.Volume = 0.8
	a.scrollVolume(ui.InputEvent{Kind: ui.InputScroll, DY: 3000})
	if math.Abs(a.settings.Volume-0.5) > 1e-9 {
		t.Fatalf("settings keep volume %v, want 0.5", a.settings.Volume)
	}
	a.toggleMute()
	if a.settings.Volume != 0 {
		t.Fatalf("muted, the settings keep %v", a.settings.Volume)
	}
	if a.volumeSave == nil {
		t.Fatal("no save was set off")
	}
	a.volumeSave.Stop()
}
