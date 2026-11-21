package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/download"
	"netease-music-mygo/internal/local"
	"netease-music-mygo/internal/lyrics"
)

// A file of the listener's own, found by its name and artist, gets the
// lyrics NetEase has of the song, word by word where it times them so.
func TestLyricsForSongLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "韩雪 - 想起.mp3")
	song := local.Song{Path: path, Title: "想起", Artist: "韩雪", Duration: 272 * time.Second}

	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricSaved {
		t.Fatalf("outcome = %v, want saved", got)
	}
	text, err := os.ReadFile(download.LyricsPath(path))
	if err != nil {
		t.Fatal(err)
	}
	lines := lyrics.FromText(string(text))
	if len(lines) == 0 || !lines[0].Timed || !lyrics.HasWordTimes(string(text)) {
		t.Fatalf("lyrics are not word by word: %q", text[:min(200, len(text))])
	}
	t.Logf("%d lines, %d bytes: %s", len(lines), len(text), strings.Split(string(text), "\r\n")[0])
}
