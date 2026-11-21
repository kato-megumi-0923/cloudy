package app

import (
	"testing"

	"netease-music-mygo/internal/download"
	"netease-music-mygo/internal/netease"
)

// A file downloaded before Cloudy wrote tags is found again on NetEase by
// its title, an artist and its length; a cover version or a live take of
// another length is not taken for it.
func TestMatchDownload(t *testing.T) {
	old := searchSongs
	defer func() { searchSongs = old }()
	searchSongs = func(q string, limit int) ([]netease.Track, error) {
		return []netease.Track{
			{ID: 1, Name: "Terranova", Artists: []netease.Artist{{Name: "Someone Else"}}, Duration: 233000},
			{ID: 2, Name: "Terranova (Live)", Artists: []netease.Artist{{Name: "Janina"}}, Duration: 233000},
			{ID: 3, Name: "terranova", Artists: []netease.Artist{{Name: "Janina"}}, Duration: 260000},
			{ID: 4, Name: "Terranova", Artists: []netease.Artist{{Name: "Janina"}}, Duration: 234500},
		}, nil
	}
	file := netease.Track{ID: -5, Name: "Terranova", Artists: []netease.Artist{{Name: "Janina"}}, Duration: 233000}
	got, ok := matchDownload(file)
	if !ok || got.ID != 4 {
		t.Fatalf("matched %v %v, want song 4", got.ID, ok)
	}
	file.Artists = []netease.Artist{{Name: "未知歌手"}}
	if _, ok := matchDownload(file); ok {
		t.Error("matched a file whose artist is unknown")
	}
}

// A file the listener put in the folder is never written to when it is
// found on NetEase: it keeps its own cover and names, and takes the song's
// ID for its lyrics. Cloudy's own download, untagged, is tagged.
func TestFixDownloadLeavesListenersFiles(t *testing.T) {
	oldSearch, oldTag := searchSongs, tagDownload
	defer func() { searchSongs, tagDownload = oldSearch, oldTag }()
	searchSongs = func(q string, limit int) ([]netease.Track, error) {
		return []netease.Track{{ID: 7, Name: "Espresso", Artists: []netease.Artist{{Name: "Sabrina Carpenter"}},
			Album: netease.Album{Name: "Espresso", PicURL: "https://p1.music.126.net/x.jpg"}, Duration: 175000}}, nil
	}
	var tagged []string
	tagDownload = func(it download.Item) bool { tagged = append(tagged, it.Path); return true }

	own := download.Item{Path: `D:\music\espresso.mp3`, Track: netease.Track{ID: -3, Name: "Espresso",
		Artists: []netease.Artist{{Name: "Sabrina Carpenter"}}, Album: netease.Album{Name: "未知专辑", PicURL: `C:\covers\mine.jpg`}, Duration: 175200}}
	got, ok := fixDownload(own)
	if !ok || !got.Matched || got.Tagged || got.Track.ID != 7 {
		t.Fatalf("own file: ok %v matched %v tagged %v id %d", ok, got.Matched, got.Tagged, got.Track.ID)
	}
	if got.Track.Album.PicURL != `C:\covers\mine.jpg` || got.Track.Album.Name != "Espresso" {
		t.Errorf("own file shows cover %q album %q, want its own cover and NetEase's album name", got.Track.Album.PicURL, got.Track.Album.Name)
	}
	if len(tagged) != 0 {
		t.Fatalf("wrote tags into the listener's file: %v", tagged)
	}

	mine := download.Item{Path: `D:\Cloudy\a.mp3`, Level: "exhigh", Track: netease.Track{ID: 9, Name: "A"}}
	if got, ok := fixDownload(mine); !ok || !got.Tagged || len(tagged) != 1 {
		t.Fatalf("Cloudy's download: ok %v tagged %v writes %v", ok, got.Tagged, tagged)
	}
	if _, ok := fixDownload(download.Item{Path: `D:\x.mp3`, Track: netease.Track{ID: 9}}); ok || len(tagged) != 1 {
		t.Error("touched a file with a NetEase ID that Cloudy did not download")
	}
}
