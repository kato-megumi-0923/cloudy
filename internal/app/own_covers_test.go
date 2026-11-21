package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"netease-music-mygo/internal/download"
	"netease-music-mygo/internal/netease"
)

func testPNG(t *testing.T) []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	return b.Bytes()
}

// A cover chosen for a song stays in Cloudy: the file is not written to,
// the song shows it through a rescan, and the one before comes back.
func TestOwnCoverForSong(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	dir := t.TempDir()
	a.settings.path = filepath.Join(dir, "settings.json")
	song := filepath.Join(dir, "music", "a.mp3")
	os.MkdirAll(filepath.Dir(song), 0o700)
	os.WriteFile(song, []byte("audio"), 0o644)
	a.dl.items = []download.Item{{Path: song, Track: netease.Track{ID: -1, Name: "A", Album: netease.Album{PicURL: "C:/embedded.jpg"}}}}
	key := ownSongKey(song)
	if err := a.setOwnCover(key, song, testPNG(t), ".png"); err != nil {
		t.Fatal(err)
	}
	own := a.dl.items[0].Track.Album.PicURL
	if own == "C:/embedded.jpg" || filepath.Dir(own) != a.ownCoversDir() {
		t.Fatalf("song shows %q", own)
	}
	if b, _ := os.ReadFile(song); string(b) != "audio" {
		t.Fatal("the song's file was written to")
	}
	// Read again from the folder, it shows the listener's still.
	a.dl.items = []download.Item{{Path: song, Track: netease.Track{ID: -1, Album: netease.Album{PicURL: "C:/embedded.jpg"}}}}
	a.ownCovers = ownCovers{}
	a.noteDownloads()
	if a.dl.items[0].Track.Album.PicURL != own {
		t.Fatalf("after a rescan the song shows %q", a.dl.items[0].Track.Album.PicURL)
	}
	if len(a.ownCoverActions(key, song)) != 2 {
		t.Fatal("no row to have the old cover back")
	}
	a.resetOwnCover(key, song)
	if got := a.dl.items[0].Track.Album.PicURL; got != "C:/embedded.jpg" {
		t.Fatalf("reset to %q", got)
	}
	if _, err := os.Stat(own); err == nil {
		t.Error("the copy stayed")
	}
}

// A playlist's chosen cover is the one it shows everywhere.
func TestOwnCoverForPlaylist(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.useOwnCovers()
	defer func() { netease.PlaylistCover = nil }()
	pl := &netease.Playlist{ID: 42, CoverURL: "https://p1.music.126.net/x.jpg"}
	if err := a.setOwnCover(ownPlaylistKey(42), "", testPNG(t), ".png"); err != nil {
		t.Fatal(err)
	}
	if got := pl.Cover(); filepath.Dir(got) != a.ownCoversDir() {
		t.Fatalf("playlist shows %q", got)
	}
	a.resetOwnCover(ownPlaylistKey(42), "")
	if pl.Cover() != pl.CoverURL {
		t.Fatal("not back to NetEase's")
	}
}
