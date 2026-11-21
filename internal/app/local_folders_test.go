package app

import "testing"

// The button names the folder it opens, cut short to fit, and counts the
// ones beside it.
func TestFoldersButtonText(t *testing.T) {
	for _, c := range []struct {
		dirs []string
		want string
	}{
		{nil, "文件夹"},
		{[]string{`C:\Music\歌`}, "歌"},
		{[]string{`C:\Music\Cloudy`}, "Cloud…"},
		{[]string{`C:\Music\我收藏的音乐很好听`}, "我收藏的音…"},
		{[]string{`C:\Music\Cloudy`, `D:\Songs`}, "Cloud… +1"},
		{[]string{`C:\Music\我收藏的音乐很好听`, `D:\Songs`, `E:\More`}, "我收藏的音… +2"},
	} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.LocalFolders = c.dirs
		if got := a.foldersButtonText(); got != c.want {
			t.Errorf("%v: %q, want %q", c.dirs, got, c.want)
		}
	}
}
