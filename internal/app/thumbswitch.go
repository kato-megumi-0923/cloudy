package app

// The window events switcherOpen reads.
const (
	eventSystemForeground  = 0x0003
	eventSystemSwitchStart = 0x0014
	eventSystemSwitchEnd   = 0x0015
	eventObjectShow        = 0x8002
	eventObjectHide        = 0x8003
)

// switcherOpen distinguishes the task switcher from the taskbar flyouts.
// Explorer also uses XAML islands for those flyouts and staging windows
// for ordinary focus changes. Neither should remove the taskbar cover.
func switcherOpen(open bool, event uint32, cls string, titled, foreground, visible, shortcut bool) bool {
	island := cls == "XamlExplorerHostIslandWindow" && titled
	classic := cls == "MultitaskingViewFrame"
	switch event {
	case eventSystemSwitchStart:
		return true
	case eventSystemSwitchEnd:
		return false
	case eventSystemForeground:
		switch {
		case cls == "ForegroundStaging":
			// The switcher takes focus just after staging, and DWM can ask
			// for thumbnails in between. Stage only a switch gesture (or
			// the tail end of an already open switcher), not a normal click.
			return open || shortcut
		case island:
			// Windows 11 may focus the named switcher before showing it,
			// and briefly focus it again after hiding it. SHOW settles the
			// first case; the second must not open it again.
			return visible || open && shortcut
		case classic:
			return true
		}
		return false
	case eventObjectShow:
		return open || (island || classic) && foreground
	case eventObjectHide:
		return open && !island && !classic
	}
	return open
}
