package netease

import (
	"strings"
	"testing"

	"netease-music-mygo/internal/lyrics"
)

// NetEase's "no lyrics" line is no lyrics; a song of one real line is.
func TestPlaceholderLyrics(t *testing.T) {
	for _, s := range []string{"", "[00:00.00]暂无歌词", "  [00:00.000] 暂无歌词 \n\n", "[by:x]\n[00:00.00]暂无歌词"} {
		if !placeholderLyrics(s) {
			t.Errorf("%q is lyrics", s)
		}
	}
	for _, s := range []string{"[00:01.00]暂无歌词\n[00:05.00]下一句", "[00:00.00]纯音乐，请欣赏", "[00:03.00]Hello"} {
		if placeholderLyrics(s) {
			t.Errorf("%q is taken for none", s)
		}
	}
}

// KRC from the drive reads as YRC, each word at its time in the song.
func TestKRCToYRC(t *testing.T) {
	krc := "[ti:x]\n[1000,2000]<0,500,0>Hel<500,1500,0>lo\n[4000,1000]<0,1000,0>world\n"
	yrc := krcToYRC(krc)
	if !strings.Contains(yrc, "[1000,2000](1000,500,0)Hel(1500,1500,0)lo") {
		t.Fatalf("yrc %q", yrc)
	}
	lines := lyrics.Build("", "", yrc)
	if len(lines) != 2 || lines[0].Text != "Hello" || lines[1].Text != "world" {
		t.Fatalf("lines %+v", lines)
	}
	if krcToYRC("not base64 krc") != "" {
		t.Error("junk read as KRC")
	}
}

// The listener's ID is asked once a session, and not without one.
func TestCloudUserAskedOnce(t *testing.T) {
	old := accountOf
	defer func() { accountOf = old }()
	asked := 0
	accountOf = func() (*Profile, error) { asked++; return &Profile{UserID: 42}, nil }
	ClearSession()
	if cloudUserID() != 0 || asked != 0 {
		t.Fatal("asked with no session")
	}
	httpClient.Jar.SetCookies(siteURL, parseCookie("MUSIC_U=abc"))
	defer ClearSession()
	if cloudUserID() != 42 || cloudUserID() != 42 || asked != 1 {
		t.Fatalf("asked %d times", asked)
	}
}
