package app

import "testing"

// The events Windows 11 (26200) sent as Alt+Tab opened and closed, and as
// the taskbar showed a thumbnail, as a probe recorded them for 1.3.3.
func TestSwitcherOpenFollowsWindows11(t *testing.T) {
	const island = "XamlExplorerHostIslandWindow"
	type ev struct {
		event            uint32
		cls              string
		titled, fore     bool
		visible, pressed bool
		open             bool // what it should be after
	}
	for name, evs := range map[string][]ev{
		"alt-tab": {
			{eventSystemForeground, "ForegroundStaging", false, true, false, true, true},
			{eventSystemForeground, island, true, true, false, true, true},
			{eventObjectShow, island, true, true, true, false, true},
			{eventObjectHide, island, false, false, false, false, true}, // an unrelated flyout
			{eventSystemForeground, "ForegroundStaging", false, true, false, false, true},
			{eventSystemForeground, "Chrome_WidgetWin_1", true, true, true, false, false},
			{eventObjectHide, island, true, false, false, false, false},
			{eventSystemForeground, island, true, true, false, false, false},
		},
		"visible switcher taking focus": {
			{eventObjectShow, island, true, false, true, false, false},
			{eventSystemForeground, island, true, true, true, false, true},
			{eventObjectHide, island, true, true, false, false, false},
		},
		"taskbar thumbnail": {
			{eventSystemForeground, "ForegroundStaging", false, true, false, false, false},
			{eventObjectShow, island, false, true, true, false, false},
			{eventObjectShow, "Xaml_WindowedPopupClass", true, true, true, false, false},
			{eventSystemForeground, island, false, true, true, false, false},
			{eventObjectHide, island, false, false, false, false, false},
		},
		"windows 10": {
			{eventSystemSwitchStart, "", false, false, false, false, true},
			{eventSystemForeground, "MultitaskingViewFrame", true, true, true, false, true},
			{eventSystemSwitchEnd, "", false, false, false, false, false},
		},
	} {
		t.Run(name, func(t *testing.T) {
			open := false
			for i, e := range evs {
				open = switcherOpen(open, e.event, e.cls, e.titled, e.fore, e.visible, e.pressed)
				if open != e.open {
					t.Fatalf("after event %d (%#x %s) open=%v, want %v", i, e.event, e.cls, open, e.open)
				}
			}
		})
	}
}
