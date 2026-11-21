package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/download"
	"netease-music-mygo/internal/local"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/wordlyrics"
)

// wordTimedYRC is a song word by word, as NetEase serves it, with its
// translation beside it.
const wordTimedYRC = "[1000,3000](1000,400,0)想(1400,350,0)起(1750,2250,0)你\n" +
	"[5000,2000](5000,500,0)我(5500,1500,0)在\n"

func stubLyrics(t *testing.T, lrc, trans, yrc string) {
	t.Helper()
	old := downloadLyricsOf
	t.Cleanup(func() { downloadLyricsOf = old })
	downloadLyricsOf = func(int64) (string, string, string, error) { return lrc, trans, yrc, nil }
}

func stubMoreWords(t *testing.T, lines []lyrics.Line) {
	t.Helper()
	old, was := findMoreWords, moreWordsOn.Load()
	oldStrict := findMoreWordsStrict
	t.Cleanup(func() {
		findMoreWords = old
		moreWordsOn.Store(was)
		findMoreWordsStrict = oldStrict
	})
	moreWordsOn.Store(true)
	find := func(context.Context, wordlyrics.Song) ([]lyrics.Line, string) {
		return lines, "AMLL"
	}
	findMoreWords = find
	findMoreWordsStrict = func(ctx context.Context, s wordlyrics.Song) ([]lyrics.Line, string, error) {
		got, from := find(ctx, s)
		return got, from, nil
	}
}

func stubSearch(t *testing.T, tracks ...netease.Track) {
	t.Helper()
	old, oldAnon := searchTracks, searchAnonTracks
	t.Cleanup(func() { searchTracks, searchAnonTracks = old, oldAnon })
	searchTracks = func(string, int) ([]netease.Track, error) { return tracks, nil }
	searchAnonTracks = func(string, int) ([]netease.Track, error) { return tracks, nil }
}

// A song known by its NetEase ID gets its words' times and its
// translation written beside it, as enhanced LRC.
func TestLyricsForSongWritesWordsAndTranslation(t *testing.T) {
	stubLyrics(t, "[00:01.00]想起你\n[00:05.00]我在\n", "[00:01.00]I think of you\n", wordTimedYRC)
	path := filepath.Join(t.TempDir(), "想起.mp3")
	song := local.Song{Path: path, Title: "想起", Artist: "韩雪", NeteaseID: 239026}

	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricSaved {
		t.Fatalf("outcome = %v, want saved", got)
	}
	text, err := os.ReadFile(download.LyricsPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if !lyrics.HasWordTimes(string(text)) || !strings.Contains(string(text), "I think of you") {
		t.Fatalf("file = %q", text)
	}
	lines := lyrics.FromText(string(text))
	if len(lines) != 2 || !lines[0].Timed || len(lines[0].Words) != 3 {
		t.Fatalf("read back %+v", lines)
	}
	if lines[0].Text != "想起你" || lines[0].Translation != "I think of you" {
		t.Fatalf("read back line %+v", lines[0])
	}
	if lines[0].Words[2].Start != 1750*time.Millisecond || lines[0].End != 4000*time.Millisecond {
		t.Fatalf("read back words %+v", lines[0].Words)
	}
}

// A file already timed word by word is left alone: nothing better is
// known, and a batch must not spoil it.
func TestLyricsForSongKeepsWordTimedFile(t *testing.T) {
	stubLyrics(t, "[00:01.00]想起你\n", "", "")
	path := filepath.Join(t.TempDir(), "想起.mp3")
	before := "[00:01.000]<00:01.000>想起你<00:04.000>\r\n"
	if err := os.WriteFile(download.LyricsPath(path), []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	song := local.Song{Path: path, Title: "想起", NeteaseID: 7}

	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricKept {
		t.Fatalf("outcome = %v, want kept", got)
	}
	text, _ := os.ReadFile(download.LyricsPath(path))
	if string(text) != before {
		t.Fatalf("file changed to %q", text)
	}
}

// A plain .lrc is replaced when another source times the song word by
// word, and the count says it was an upgrade.
func TestLyricsForSongUpgradesPlainLyrics(t *testing.T) {
	stubLyrics(t, "[00:01.00]想起你\n", "", "")
	stubMoreWords(t, []lyrics.Line{{
		At: time.Second, End: 4 * time.Second, Text: "想起你", Timed: true,
		Words: []lyrics.Word{
			{Text: "想", Start: time.Second, End: 1400 * time.Millisecond},
			{Text: "起", Start: 1400 * time.Millisecond, End: 1750 * time.Millisecond},
			{Text: "你", Start: 1750 * time.Millisecond, End: 4 * time.Second},
		},
	}})
	path := filepath.Join(t.TempDir(), "想起.mp3")
	if err := os.WriteFile(download.LyricsPath(path), []byte("[00:01.00]想起你\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	song := local.Song{Path: path, Title: "想起", NeteaseID: 7}

	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricUpgraded {
		t.Fatalf("outcome = %v, want upgraded", got)
	}
	text, _ := os.ReadFile(download.LyricsPath(path))
	if !lyrics.HasWordTimes(string(text)) {
		t.Fatalf("file = %q", text)
	}
	// Only a batch that replaces what is there may do so.
	if err := os.WriteFile(download.LyricsPath(path), []byte("[00:01.00]想起你\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lyricsForSong(context.Background(), song, lyricsMissing); got != lyricKept {
		t.Fatalf("outcome = %v, want kept when only missing are wanted", got)
	}
}

// A song with no lyrics anywhere is left without a file.
func TestLyricsForSongWithoutLyrics(t *testing.T) {
	stubLyrics(t, "", "", "")
	path := filepath.Join(t.TempDir(), "无名.mp3")
	song := local.Song{Path: path, Title: "无名", NeteaseID: 7}

	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricNoLyrics {
		t.Fatalf("outcome = %v, want no lyrics", got)
	}
	if _, err := os.Stat(download.LyricsPath(path)); !os.IsNotExist(err) {
		t.Fatal("a file was written for a song without lyrics")
	}
}

// A file that does not say which song it is is found by its name, artist
// and length; one of another take is not.
func TestMatchLyricsTrack(t *testing.T) {
	stubSearch(t,
		netease.Track{ID: 1, Name: "想起", Artists: []netease.Artist{{Name: "江美琪"}}, Duration: 265800},
		netease.Track{ID: 2, Name: "想起", Artists: []netease.Artist{{Name: "韩雪"}}, Duration: 272867},
	)
	track, ok, err := matchLyricsTrack(context.Background(), "想起", []string{"韩雪"}, 272*time.Second)
	if err != nil || !ok || track.ID != 2 {
		t.Fatalf("matched %+v, %v, %v", track, ok, err)
	}
	if _, ok, _ := matchLyricsTrack(context.Background(), "想起", []string{"韩雪"}, 200*time.Second); ok {
		t.Fatal("matched a song of another length")
	}
	if _, ok, _ := matchLyricsTrack(context.Background(), "晴天", []string{"韩雪"}, 272*time.Second); ok {
		t.Fatal("matched a song of another name")
	}
}

// A search the network failed is no answer: the song is asked for again,
// rather than counted as one NetEase does not have.
func TestLyricsForSongRetriesAFailedSearch(t *testing.T) {
	oldDelay := lyricsRetryDelay
	lyricsRetryDelay = 0
	t.Cleanup(func() { lyricsRetryDelay = oldDelay })

	oldSearch := searchTracks
	t.Cleanup(func() { searchTracks = oldSearch })
	oldAnon := searchAnonTracks
	t.Cleanup(func() { searchAnonTracks = oldAnon })
	searchAnonTracks = func(string, int) ([]netease.Track, error) { return nil, errors.New("no network") }
	calls := 0
	searchTracks = func(string, int) ([]netease.Track, error) {
		calls++
		if calls <= 2 {
			return nil, errors.New("no network")
		}
		return []netease.Track{{ID: 7, Name: "想起", Artists: []netease.Artist{{Name: "韩雪"}}, Duration: 272000}}, nil
	}
	stubLyrics(t, "[00:01.00]想起你\n", "", "")
	stubMoreWords(t, nil)

	path := filepath.Join(t.TempDir(), "想起.mp3")
	song := local.Song{Path: path, Title: "想起", Artist: "韩雪", Duration: 272 * time.Second}
	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricSaved {
		t.Fatalf("outcome = %v, want saved after a retry", got)
	}
	if calls <= 2 {
		t.Fatalf("the search was not retried: %d calls", calls)
	}

	// Every search failing is a failure of the batch, not a song NetEase
	// does not have.
	searchTracks = func(string, int) ([]netease.Track, error) { return nil, errors.New("no network") }
	other := filepath.Join(t.TempDir(), "想起2.mp3")
	if got := lyricsForSong(context.Background(), local.Song{Path: other, Title: "想起", Artist: "韩雪"}, lyricsUpgrade); got != lyricFailed {
		t.Fatalf("outcome = %v, want failed", got)
	}
}

// The batch reads a folder of its own: a song matched by name gets its
// lyrics, and the state says how far it came.
func TestLyricsFolderJob(t *testing.T) {
	src, err := filepath.Abs(filepath.Join("..", "player", "testdata", "seek.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "测试 - 想起.mp3")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	song, err := local.Read(path)
	if err != nil || song.Title == "" {
		t.Skipf("the sample has no tags to look the song up by: %+v, %v", song, err)
	}
	track := netease.Track{ID: 7, Name: song.Title, Duration: song.Duration.Milliseconds()}
	if song.Artist != "" {
		track.Artists = []netease.Artist{{Name: song.Artist}}
	}
	stubSearch(t, track)
	stubLyrics(t, "[00:01.00]想起你\n", "", "")
	stubMoreWords(t, nil)

	a := newTestApp()
	a.settings = defaultSettings()
	a.lyricsFolder.dir = dir
	a.lyricsFolder.mode = lyricsUpgrade
	a.startLyricsFolder()

	s := &a.lyricsFolder
	if s.running || !s.finished || s.all != 1 || s.done != 1 || s.saved != 1 {
		t.Fatalf("state = %+v", s)
	}
	text, err := os.ReadFile(download.LyricsPath(path))
	if err != nil || !strings.Contains(string(text), "想起你") {
		t.Fatalf("lyrics = %q, %v", text, err)
	}
}

// Two files of the same song share one .lrc: only one of them is read, so
// that neither writes over what the other found.
func TestLyricsTargets(t *testing.T) {
	got := lyricsTargets([]string{
		`C:\m\a - b.flac`,
		`C:\m\a - b.mp3`,
		`C:\m\c.mp3`,
	})
	if len(got) != 2 || got[0] != `C:\m\a - b.flac` || got[1] != `C:\m\c.mp3` {
		t.Fatalf("targets = %q", got)
	}
}

// Line-timed lyrics never replace word-timed ones already beside a song,
// whichever file of it asked last.
func TestLyricsForSongNeverDowngrades(t *testing.T) {
	stubLyrics(t, "[00:01.00]想起你\n", "", "")
	stubMoreWords(t, nil)
	path := filepath.Join(t.TempDir(), "想起.mp3")
	before := "[00:01.000]<00:01.000>想起你<00:04.000>\r\n"
	if err := os.WriteFile(download.LyricsPath(path), []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	song := local.Song{Path: path, Title: "想起", NeteaseID: 7}

	if got := lyricsForSong(context.Background(), song, lyricsAll); got != lyricKept {
		t.Fatalf("outcome = %v, want kept", got)
	}
	text, _ := os.ReadFile(download.LyricsPath(path))
	if string(text) != before {
		t.Fatalf("word-timed lyrics were replaced: %q", text)
	}
}

// A search an account may not make is made as nobody: NetEase refuses a
// burst of them from one signed in, and answers an anonymous one.
func TestLyricsForSongFallsBackToAnonymousSearch(t *testing.T) {
	oldDelay := lyricsRetryDelay
	lyricsRetryDelay = 0
	t.Cleanup(func() { lyricsRetryDelay = oldDelay })
	oldQuiet := sessionSearchQuiet.Load()
	t.Cleanup(func() { sessionSearchQuiet.Store(oldQuiet) })
	sessionSearchQuiet.Store(0)

	oldSearch, oldAnon := searchTracks, searchAnonTracks
	t.Cleanup(func() { searchTracks, searchAnonTracks = oldSearch, oldAnon })
	searchTracks = func(string, int) ([]netease.Track, error) { return nil, netease.ErrTooFrequent }
	searchAnonTracks = func(string, int) ([]netease.Track, error) {
		return []netease.Track{{ID: 7, Name: "想起", Artists: []netease.Artist{{Name: "韩雪"}}, Duration: 272000}}, nil
	}
	stubLyrics(t, "[00:01.00]想起你\n", "", "")
	stubMoreWords(t, nil)

	path := filepath.Join(t.TempDir(), "想起.mp3")
	song := local.Song{Path: path, Title: "想起", Artist: "韩雪", Duration: 272 * time.Second}
	if got := lyricsForSong(context.Background(), song, lyricsUpgrade); got != lyricSaved {
		t.Fatalf("outcome = %v, want saved", got)
	}
	if sessionSearchQuiet.Load() == 0 {
		t.Fatal("the session's searches were not held back")
	}
}

// The dialog starts the batch for the folder it was opened with, and one
// with no songs is reported rather than left running.
func TestLyricsFolderDialog(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.lyricsFolder.dir = t.TempDir()
	a.openLyricsFolder()

	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		a.motion = motionOff
		a.lyricsFolderDialog(c)
	}, 560, 620)
	if err := tt.Click("开始下载"); err != nil {
		t.Fatal(err)
	}
	if a.lyricsFolder.running {
		t.Fatal("the batch kept running with no songs")
	}
	if !a.lyricsFolder.finished || a.lyricsFolder.err == "" {
		t.Fatalf("an empty folder was not reported: %+v", a.lyricsFolder)
	}
}
