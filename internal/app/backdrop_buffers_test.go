package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Water scenes reuse their uniform buffers, and a departed leaf leaves no stale position.
func TestWaterBackdropReusesUniformBuffers(t *testing.T) {
	var beat beatState
	var leaves leafState
	beat.drops[0] = sceneDrop{x: 0.25, y: 0.5, at: 2, hard: 1}
	leaves.leaves[0] = poolLeaf{x: 1, y: 0.5, half: 0.05, alive: true}
	var drops, shapes []float32
	if got := testing.AllocsPerRun(20, func() {
		drops, shapes = beat.uniform(), leaves.uniform(2)
	}); got != 0 {
		t.Errorf("uniforms allocate %g times per paint", got)
	}
	if drops[0] != 0.25 || drops[3] != 1 || shapes[0] != 0.5 || shapes[3] != 0.05 {
		t.Fatal("the shader uniforms lost the drop or the leaf")
	}
	leaves.leaves[0].alive = false
	if got := leaves.uniform(2); got[0] != 0 || got[3] != 0 {
		t.Fatal("the shader kept a departed leaf")
	}
}

// The static preview reuses its canvas across unrelated frames and redraws on a changed selection.
func TestStaticBackdropPreviewRefreshesOnSelection(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.nowPlayingPreview, 360, 240)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	p := &a.settingsView.preview
	original := p.at
	if original.IsZero() {
		t.Fatal("the initial preview did not draw")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if p.at != original {
		t.Error("an unrelated frame redrew the static background")
	}
	a.settings.Fog = true
	a.settings.FogStyle = "bokeh"
	tt.Frame()
	if p.at == original || p.shown.style != "bokeh" || p.shown.moving {
		t.Error("the preview did not paint the new scene as a still image")
	}
}
