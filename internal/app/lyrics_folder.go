package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/download"
	"netease-music-mygo/internal/local"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/wordlyrics"
)

// The ways a batch treats the .lrc files already beside the songs.
const (
	lyricsMissing = "missing" // only the songs without any
	lyricsUpgrade = "upgrade" // those without, and plain ones that can be word-timed
	lyricsAll     = "all"     // every one, again
)

var lyricsFolderModes = []string{lyricsMissing, lyricsUpgrade, lyricsAll}

// lyricsFolderModeLabels names each mode, in the order of lyricsFolderModes.
func lyricsFolderModeLabels() []string {
	return []string{"只补缺失的", "补齐并升级为逐字", "全部重新下载"}
}

func lyricsFolderModeIndex(mode string) int {
	if i := slices.Index(lyricsFolderModes, mode); i >= 0 {
		return i
	}
	return slices.Index(lyricsFolderModes, lyricsUpgrade)
}

// A lyricsFolderState is a batch of lyrics for a folder's songs: where it
// reads from, how it treats what is there, and how far it has come.
type lyricsFolderState struct {
	open     bool
	dir      string
	mode     string
	running  bool
	stopping bool
	finished bool
	cancel   context.CancelFunc
	all      int
	done     int
	saved    int
	upgraded int
	kept     int
	noMatch  int // no song of NetEase's is it
	noLyrics int // the song has none anywhere
	failed   int
	current  string
	err      string
}

// A lyricsOutcome says what a batch did with one song.
type lyricsOutcome int

const (
	lyricSaved    lyricsOutcome = iota // written anew
	lyricUpgraded                      // a plain .lrc replaced by a word-timed one
	lyricKept                          // left as it was
	lyricNoMatch                       // no song of NetEase's is it
	lyricNoLyrics                      // the song has none anywhere
	lyricFailed                        // reading, fetching or writing failed
	lyricStopped                       // the batch was stopped
)

// searchTracks is netease.Search; tests replace it.
var searchTracks = netease.Search

// findMoreWordsStrict is wordlyrics.FindStrict; tests replace it.
var findMoreWordsStrict = wordlyrics.FindStrict

// lyricsTargets leaves one file of each .lrc a folder would write: an mp3
// and a flac of the same song share theirs, and two workers would race for
// it, the last one written winning.
func lyricsTargets(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := paths[:0]
	for _, p := range paths {
		key := strings.ToLower(download.LyricsPath(p))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// openLyricsFolder shows the batch's dialog, starting where the library's
// last folder is.
func (a *app) openLyricsFolder() {
	s := &a.lyricsFolder
	s.open = true
	if s.mode == "" {
		s.mode = lyricsUpgrade
	}
	if s.dir == "" {
		s.dir = a.lyricsFolderStart()
	}
	a.redraw()
}

// lyricsFolderStart is the folder the batch starts at: the library's last
// one, or the one the system keeps music in.
func (a *app) lyricsFolderStart() string {
	if n := len(a.settings.LocalFolders); n > 0 {
		return a.settings.LocalFolders[n-1]
	}
	if music, err := mygo.App.Path(mygo.PathMusic); err == nil {
		return music
	}
	return ""
}

// pickLyricsFolder asks for the folder to read, for the batch's dialog.
func (a *app) pickLyricsFolder() {
	s := &a.lyricsFolder
	if s.running {
		return
	}
	start := s.dir
	if start == "" {
		start = a.lyricsFolderStart()
	}
	if a.win == nil {
		if dir, err := chooseFolder(nil, "选择要下载歌词的文件夹", start); err == nil && dir != "" {
			s.dir = dir
		}
		return
	}
	go func() {
		dir, err := chooseFolder(a.win, "选择要下载歌词的文件夹", start)
		if err != nil || dir == "" {
			return
		}
		a.post(func() { s.dir = dir })
	}()
}

// startLyricsFolder reads the folder, and finds the lyrics of its songs.
func (a *app) startLyricsFolder() {
	s := &a.lyricsFolder
	if s.running || s.dir == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.running, s.stopping, s.finished, s.err = true, false, false, ""
	s.all, s.done, s.saved, s.upgraded, s.kept, s.noMatch, s.noLyrics, s.failed = 0, 0, 0, 0, 0, 0, 0, 0
	s.current, s.cancel = "", cancel
	dir, mode := s.dir, s.mode
	if a.win == nil {
		a.runLyricsFolder(ctx, dir, mode) // no window to come back to (tests)
		return
	}
	go func() { a.runLyricsFolder(ctx, dir, mode) }()
	a.redraw()
}

// stopLyricsFolder stops the batch where it is: what is written stays.
func (a *app) stopLyricsFolder() {
	if a.lyricsFolder.cancel != nil {
		a.lyricsFolder.cancel()
	}
	a.lyricsFolder.stopping = true
	a.toast = "正在停止下载歌词"
}

// runLyricsFolder finds the lyrics of the songs in dir, a few at a time:
// each is a request to NetEase, and often to another source after it.
func (a *app) runLyricsFolder(ctx context.Context, dir, mode string) {
	s := &a.lyricsFolder
	// A worker's counts are changed on the main thread, one at a time; a
	// test has no thread to come back to and runs them where they are, so
	// that they are still one at a time.
	var mu sync.Mutex
	update := func(fn func()) {
		if a.win == nil {
			mu.Lock()
			defer mu.Unlock()
			fn()
			return
		}
		a.post(fn)
	}
	paths, err := local.Paths(dir)
	if err != nil {
		update(func() {
			s.running, s.stopping, s.finished, s.cancel = false, false, true, nil
			s.err = "无法读取文件夹：" + err.Error()
		})
		return
	}
	paths = lyricsTargets(paths)
	update(func() { s.all = len(paths) })

	work := make(chan string)
	var wg sync.WaitGroup
	for range min(4, max(1, len(paths))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				if ctx.Err() != nil {
					return
				}
				outcome := lyricsForPath(ctx, path, mode)
				name := filepath.Base(path)
				update(func() {
					s.done++
					s.current = name
					switch outcome {
					case lyricSaved:
						s.saved++
					case lyricUpgraded:
						s.upgraded++
					case lyricKept:
						s.kept++
					case lyricNoMatch:
						s.noMatch++
					case lyricNoLyrics:
						s.noLyrics++
					case lyricFailed:
						s.failed++
					}
				})
			}
		}()
	}
feed:
	for _, path := range paths {
		select {
		case work <- path:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()
	stopped := ctx.Err() != nil

	update(func() {
		s.running, s.stopping, s.finished, s.cancel = false, false, true, nil
		s.current = ""
		if n := len(paths); n == 0 {
			s.err = "这个文件夹里没有找到 MP3、FLAC 或 WAV 文件。"
			return
		}
		parts := []string{fmt.Sprintf("已保存 %d 首", s.saved)}
		if s.upgraded > 0 {
			parts = append(parts, fmt.Sprintf("升级 %d 首", s.upgraded))
		}
		if s.kept > 0 {
			parts = append(parts, fmt.Sprintf("已有 %d 首", s.kept))
		}
		if s.noLyrics > 0 {
			parts = append(parts, fmt.Sprintf("无歌词 %d 首", s.noLyrics))
		}
		if s.noMatch > 0 {
			parts = append(parts, fmt.Sprintf("没找到 %d 首", s.noMatch))
		}
		if s.failed > 0 {
			parts = append(parts, fmt.Sprintf("失败 %d 首", s.failed))
		}
		summary := strings.Join(parts, "，")
		if stopped {
			summary = "已停止，" + summary
		}
		a.toast, a.toastAs = summary, "lyrics-folder"
	})
}

// lyricsForPath reads the song at path, and finds its lyrics.
func lyricsForPath(ctx context.Context, path, mode string) lyricsOutcome {
	song, err := local.Read(path)
	if err != nil {
		return lyricFailed
	}
	return lyricsForSong(ctx, *song, mode)
}

// lyricsForSong writes the lyrics of song beside it, as mode wants them:
// word-timed where NetEase or another source times them word by word, and
// with the translation where there is one. A song without lyrics is left
// alone; so is a file that already has better ones.
func lyricsForSong(ctx context.Context, song local.Song, mode string) lyricsOutcome {
	if song.Path == "" {
		return lyricFailed
	}
	target := download.LyricsPath(song.Path)
	old, _ := os.ReadFile(target)
	existing := strings.TrimSpace(string(old))
	switch mode {
	case lyricsMissing:
		if existing != "" {
			return lyricKept
		}
	case lyricsUpgrade:
		if lyrics.HasWordTimes(existing) {
			return lyricKept
		}
	}
	if ctx.Err() != nil {
		return lyricStopped
	}

	title, artists := song.Title, splitArtists(song.Artist)
	length, id := song.Duration, song.NeteaseID
	if id == 0 {
		track, ok, err := matchLyricsTrack(ctx, title, artists, length)
		if err != nil {
			if ctx.Err() != nil {
				return lyricStopped
			}
			return lyricFailed
		}
		if !ok {
			return lyricNoMatch
		}
		id, title, artists, length = track.ID, track.Name, trackArtists(track), track.Length()
	}

	lrc, trans, yrc, err := downloadLyricsOf(id)
	if err != nil && lrc == "" && yrc == "" && ctx.Err() == nil {
		lyricsPause(ctx, 1)
		lrc, trans, yrc, err = downloadLyricsOf(id)
	}
	lines := lyrics.Build(lrc, trans, yrc)
	if len(lines) == 0 {
		if err != nil {
			return lyricFailed
		}
		return lyricNoLyrics
	}
	// Songs NetEase times only by line may be timed word by word elsewhere;
	// the translation it has still goes with them. The setting that lets the
	// player look elsewhere holds here too.
	if !hasTimedLines(lines) && moreWordsOn.Load() {
		want := wordlyrics.Song{ID: id, Name: title, Artists: artists, Length: length}
		more, _, err := findMoreWordsStrict(ctx, want)
		if err != nil && ctx.Err() == nil {
			// A source that could not be asked is no answer: once more.
			lyricsPause(ctx, 1)
			more, _, _ = findMoreWordsStrict(ctx, want)
		}
		if len(more) > 0 {
			lines = lyrics.KeepTranslations(more, lines)
		}
	}
	text := lyrics.ToLRC(lines)
	if text == "" {
		return lyricNoLyrics
	}
	if text == string(old) {
		return lyricKept
	}
	// Another file of the same song may have written better lyrics since
	// this one was read: never put line-timed ones over word-timed ones.
	if now, err := os.ReadFile(target); err == nil {
		if cur := string(now); lyrics.HasWordTimes(cur) && !lyrics.HasWordTimes(text) {
			return lyricKept
		}
	}
	if err := writeFileAtomic(target, []byte(text), 0o644); err != nil {
		return lyricFailed
	}
	if existing != "" && !lyrics.HasWordTimes(existing) && lyrics.HasWordTimes(text) {
		return lyricUpgraded
	}
	return lyricSaved
}

// matchLyricsTrack finds the NetEase song of a file that does not say
// which it is: of the same name, by one of its artists, and within five
// seconds of its length. err says the search itself failed, which is not
// the same as the song being none of NetEase's.
func matchLyricsTrack(ctx context.Context, title string, artists []string, length time.Duration) (netease.Track, bool, error) {
	if strings.TrimSpace(title) == "" {
		return netease.Track{}, false, nil
	}
	var queries []string
	if len(artists) > 0 {
		queries = append(queries, strings.Join(artists, " ")+" "+title)
	}
	queries = append(queries, title)
	searched := false
	for _, q := range queries {
		if err := ctx.Err(); err != nil {
			return netease.Track{}, false, err
		}
		tracks, err := searchWithRetry(ctx, q)
		if err != nil {
			continue
		}
		searched = true
		for _, t := range tracks {
			if t.ID == 0 || t.ProgramID != 0 {
				continue
			}
			if wordlyrics.Fold(t.Name) != wordlyrics.Fold(title) {
				continue
			}
			if length > 0 && t.Duration > 0 {
				if gap := t.Length() - length; gap < -5*time.Second || gap > 5*time.Second {
					continue
				}
			}
			if len(artists) > 0 && !wordlyrics.SharesArtist(t.ArtistNames(), artists) {
				continue
			}
			return t, true, nil
		}
	}
	if !searched {
		return netease.Track{}, false, errSearchFailed
	}
	return netease.Track{}, false, nil
}

// errSearchFailed says no search of a song could be made: the network, not
// the song, is why nothing matched.
var errSearchFailed = errors.New("lyrics: search failed")

// lyricsRetryDelay is how long a batch waits before asking again what the
// network did not answer; tests shorten it.
var lyricsRetryDelay = 400 * time.Millisecond

func lyricsPause(ctx context.Context, attempt int) {
	select {
	case <-ctx.Done():
	case <-time.After(lyricsRetryDelay * time.Duration(attempt)):
	}
}

// searchAnonTracks is netease.SearchAnonymous; tests replace it.
var searchAnonTracks = netease.SearchAnonymous

// searchTries is how many times a search the network refused is asked again.
const searchTries = 3

// sessionSearchQuiet is when the batch may ask the signed-in search API
// again, in Unix nanoseconds: it refuses a burst of them, and asks for a
// quiet while before taking more.
var sessionSearchQuiet atomic.Int64

// searchWithRetry Searches as the listener, and for the rest of the batch
// as nobody once NetEase has had enough of them: an anonymous search is
// answered where an account's is refused.
func searchWithRetry(ctx context.Context, q string) ([]netease.Track, error) {
	var err error
	if time.Now().UnixNano() >= sessionSearchQuiet.Load() {
		for attempt := 1; attempt <= searchTries; attempt++ {
			var tracks []netease.Track
			tracks, err = searchTracks(q, 8)
			if err == nil {
				return tracks, nil
			}
			if errors.Is(err, netease.ErrTooFrequent) {
				sessionSearchQuiet.Store(time.Now().Add(time.Minute).UnixNano())
				break
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt < searchTries {
				lyricsPause(ctx, attempt)
			}
		}
	}
	tracks, anonErr := searchAnonTracks(q, 8)
	if anonErr != nil {
		return nil, anonErr
	}
	return tracks, nil
}

// splitArtists reads the artists a tag names, however it joins them.
func splitArtists(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '/', '\\', '、', ';', '；', '&', ',', '，':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func trackArtists(t netease.Track) []string {
	out := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if a.Name != "" {
			out = append(out, a.Name)
		}
	}
	return out
}

func hasTimedLines(lines []lyrics.Line) bool {
	for _, l := range lines {
		if l.Timed {
			return true
		}
	}
	return false
}

// lyricsFolderDialog is the batch's dialog: where it reads from, what it
// does with the lyrics already there, and how far it has come.
func (a *app) lyricsFolderDialog(c *ui.Context) {
	s := &a.lyricsFolder
	a.dialog(c, "lyrics-folder", 0.25, &s.open, func(panel ui.Element) {
		ink := a.inkOn(a.nowPlaying)
		p := a.pal
		panel.Width(520).Clip().TextColor(ink.text).Label("批量下载歌词")
		a.pane(panel, 26, a.nowPlaying)
		panel.Children(func() {
			dialogHeader(c, ink, "批量下载歌词", "为文件夹里的歌曲找逐字歌词，有翻译的一起保存为同名 .lrc。", func() {
				if !s.running {
					s.open = false
				}
			})
			ui.Column(c.Key("body")).PaddingX(28).Gap(16).Children(func() {
				ui.Row(c.Key("folder-row")).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(14).Children(func() {
					ui.Column(c.Key("folder-name")).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, "文件夹").FontSize(13.5).FontWeight(500)
						name, sub := "尚未选择", "选一个存放音乐的文件夹"
						if s.dir != "" {
							name, sub = filepath.Base(s.dir), s.dir
						}
						ui.Text(c, name).FontSize(13).SingleLine().TextColor(ink.text)
						ui.Text(c, sub).FontSize(11.5).SingleLine().TextColor(ink.faint)
					})
					if settingButton(c.Key("pick-folder"), ink, "选择…", iconFolder, s.running, false).Clicked() && !s.running {
						a.pickLyricsFolder()
					}
				})
				ui.Row(c.Key("mode-row")).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "已有歌词").FontSize(13.5)
					labels := lyricsFolderModeLabels()
					chosen := labels[lyricsFolderModeIndex(s.mode)]
					if a.paneSelect(c.Key("mode"), "如何处理已有歌词", &chosen, labels) {
						if i := slices.Index(labels, chosen); i >= 0 {
							s.mode = lyricsFolderModes[i]
						}
					}
				})
				ui.Text(c.Key("hint"), lyricsFolderHint(s.mode)).FontSize(12).LineHeight(1.5).TextColor(ink.muted)
				if !moreWordsOn.Load() {
					ui.Text(c.Key("words-off"), "设置里的「其他来源逐字歌词」已关闭：只保存网易云的歌词，只有逐行的就存逐行的。").
						FontSize(12).LineHeight(1.5).TextColor(ink.muted)
				}
				if s.running || s.finished {
					a.lyricsFolderProgress(c, ink)
				}
			})
			dialogFooter(c, func() {
				if s.running {
					if settingButton(c.Key("stop"), ink, "停止", nil, s.stopping, false).Clicked() && !s.stopping {
						a.stopLyricsFolder()
					}
					primaryButton(c.Key("start"), "正在下载", iconDownload, p, true)
					return
				}
				if settingButton(c.Key("close"), ink, "关闭", nil, false, false).Clicked() {
					s.open = false
				}
				if primaryButton(c.Key("start"), "开始下载", iconDownload, p, s.dir == "").Clicked() && s.dir != "" {
					a.startLyricsFolder()
				}
			})
		})
	})
}

// lyricsFolderHint says what the mode chosen does.
func lyricsFolderHint(mode string) string {
	switch mode {
	case lyricsMissing:
		return "只为还没有 .lrc 的歌曲下载。已有的歌词不会被改动。"
	case lyricsAll:
		return "重新下载每一首歌的歌词，覆盖已有的 .lrc。"
	default:
		return "补齐没有歌词的歌曲；已经有 .lrc 的，只有在找到逐字歌词时才替换，逐行换成逐字。"
	}
}

// lyricsFolderProgress shows how far the batch has come, and what it did.
func (a *app) lyricsFolderProgress(c *ui.Context, ink paneInk) {
	s := &a.lyricsFolder
	share := float32(0)
	if s.all > 0 {
		share = min(float32(s.done)/float32(s.all), 1)
	}
	head := fmt.Sprintf("已完成 %d/%d", s.done, s.all)
	if s.finished {
		head = fmt.Sprintf("完成 %d/%d", s.done, s.all)
	} else if s.stopping {
		head = "正在停止…"
	}
	ui.Column(c.Key("progress")).Gap(6).Margin(2, 0, 0, 0).Children(func() {
		ui.Row(c.Key("progress-head")).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, head).FontSize(12).FontWeight(600).TextColor(ink.text).SingleLine()
			if s.current != "" {
				ui.Text(c, s.current).FontSize(11.5).TextColor(ink.muted).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			}
		})
		bar := ui.Box(c.Key("progress-bar")).Height(5).Radius(3).Background(ink.hover)
		bar.Draw(func(pt *ui.Painter, r ui.Rect) {
			if share > 0 {
				pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * share, H: r.H}, a.pal.accent, 3)
			}
		})
		ui.Text(c, fmt.Sprintf("保存 %d · 升级 %d · 已有 %d · 失败 %d",
			s.saved, s.upgraded, s.kept, s.failed)).FontSize(11.5).TextColor(ink.muted)
		if s.finished && (s.noMatch > 0 || s.noLyrics > 0) {
			missed := fmt.Sprintf("%d 首没有歌词", s.noLyrics)
			advice := ""
			if s.noMatch > 0 {
				missed = fmt.Sprintf("%d 首没找到网易云歌曲，%s", s.noMatch, missed)
				advice = "登录网易云账号后重试通常能找到。"
			}
			ui.Text(c, missed+"。"+advice).FontSize(11.5).LineHeight(1.5).TextColor(ink.muted)
		}
		if s.err != "" {
			ui.Text(c, s.err).FontSize(11.5).LineHeight(1.5).TextColor(ink.muted)
		}
	})
}
