package app

import (
	"syscall"
	"unsafe"
)

// Alt+Tab and Win+Tab, unlike the taskbar, show the window as itself: a
// cover in its place is drawn no larger than DWM asks for, which there is
// shorter than the windows beside it, and DWM refuses a larger one. Nothing
// tells a window which of them asks, so the cover gives way while a switcher
// is open (switcherOpen says when).

const wineventOutOfContext = 0x0000

var (
	procSetWinEventHook   = user32.NewProc("SetWinEventHook")
	procWindowTitleLength = user32.NewProc("GetWindowTextLengthW")
	procSwitcherVisible   = user32.NewProc("IsWindowVisible")
	procSwitcherKeyState  = user32.NewProc("GetAsyncKeyState")
	altTabHook            uintptr // the callback, made once
)

// watchAltTab follows Alt+Tab and Win+Tab opening and closing, on the main
// thread, whose message loop the hook's events come through.
func (a *app) watchAltTab() {
	if altTabHook != 0 {
		return
	}
	altTabHook = syscall.NewCallback(func(hook, event, hwnd, object, child, thread, at uintptr) uintptr {
		if event == eventObjectShow || event == eventObjectHide {
			// Only whole windows: a show or hide of anything in them
			// comes through too, and often.
			if object != 0 || hwnd == 0 {
				return 0
			}
			if top, _, _ := procGetAncestor.Call(hwnd, 2); top != hwnd {
				return 0
			}
		}
		cls := ""
		var titled, visible, foreground, shortcut bool
		if event != eventSystemSwitchStart && event != eventSystemSwitchEnd {
			cls = windowClass(hwnd)
			if cls == "XamlExplorerHostIslandWindow" {
				width, _, _ := procWindowTitleLength.Call(hwnd)
				titled = width != 0
				if titled && event == eventSystemForeground {
					shown, _, _ := procSwitcherVisible.Call(hwnd)
					visible = shown != 0
				}
			}
			if event == eventObjectShow && (titled || cls == "MultitaskingViewFrame") {
				fore, _, _ := procGetForegroundWnd.Call()
				foreground = fore == hwnd
			}
			if event == eventSystemForeground && (cls == "ForegroundStaging" || titled && !visible) {
				shortcut = switcherModifierHeld()
			}
		}
		open := switcherOpen(a.altTab, uint32(event), cls, titled, foreground, visible, shortcut)
		if open != a.altTab {
			a.altTab = open
			a.syncThumbs()
		}
		return 0
	})
	procSetWinEventHook.Call(eventSystemForeground, eventSystemForeground, 0, altTabHook, 0, 0, wineventOutOfContext)
	procSetWinEventHook.Call(eventSystemSwitchStart, eventSystemSwitchEnd, 0, altTabHook, 0, 0, wineventOutOfContext)
	procSetWinEventHook.Call(eventObjectShow, eventObjectHide, 0, altTabHook, 0, 0, wineventOutOfContext)
}

func switcherModifierHeld() bool {
	for _, vk := range [...]uintptr{0x12, 0x5b, 0x5c} { // Alt, left Windows, right Windows
		if state, _, _ := procSwitcherKeyState.Call(vk); state&0x8000 != 0 {
			return true
		}
	}
	return false
}

func windowClass(hwnd uintptr) string {
	var buf [64]uint16
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:])
}
