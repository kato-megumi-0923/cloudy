package wordlyrics

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/lyrics"
)

// qqKey encrypts QQ's lyrics. It is QQ's own, and the cipher above is too.
var qqKey = []byte("!@#)(*$%123ZXC!@!@#)(NHL")

const (
	qqSearchURL = "https://c.y.qq.com/soso/fcgi-bin/client_search_cp"
	qqLyricURL  = "https://u.y.qq.com/cgi-bin/musicu.fcg"
	// qqTolerance is how far a candidate may run from the file it is for.
	qqTolerance = 3 * time.Second
	qqAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
)

// qqSong is a song as QQ's search lists it.
type qqSong struct {
	Mid      string `json:"songmid"`
	Name     string `json:"songname"`
	Interval int    `json:"interval"` // 秒
	Singers  []struct {
		Name string `json:"name"`
	} `json:"singer"`
}

func (s qqSong) artists() []string {
	out := make([]string, 0, len(s.Singers))
	for _, a := range s.Singers {
		if a.Name != "" {
			out = append(out, a.Name)
		}
	}
	return out
}

// qqSearch lists what QQ calls keyword, songs only.
func qqSearch(ctx context.Context, keyword string) ([]qqSong, error) {
	q := url.Values{"w": {keyword}, "format": {"json"}, "n": {"20"}, "p": {"1"}, "t": {"0"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qqSearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", qqAgent)
	req.Header.Set("Referer", "https://y.qq.com/")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wordlyrics: qq search answered %d", res.StatusCode)
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			Song struct {
				List []qqSong `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("wordlyrics: qq search code %d", out.Code)
	}
	return out.Data.Song.List, nil
}

// qqEncrypted is the encrypted QRC of a song, as the lyric API hands it
// over: hexadecimal, or 0 when QQ has none.
func qqEncrypted(ctx context.Context, mid string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"comm": map[string]any{"uin": "0", "format": "json", "ct": 11, "cv": "1003006"},
		"req_1": map[string]any{
			"module": "music.musichallSong.PlayLyricInfo",
			"method": "GetPlayLyricInfo",
			"param":  map[string]any{"songMID": mid, "songID": 0, "qrc": 1, "lrcType": 1},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, qqLyricURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", qqAgent)
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wordlyrics: qq lyric answered %d", res.StatusCode)
	}
	var out struct {
		Req struct {
			Data struct {
				Lyric json.RawMessage `json:"lyric"`
			} `json:"data"`
		} `json:"req_1"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	raw := strings.Trim(strings.TrimSpace(string(out.Req.Data.Lyric)), `"`)
	if raw == "" || raw == "0" || raw == "null" {
		return "", nil
	}
	return raw, nil
}

// decryptQRC turns QQ's encrypted lyrics into the QRC text they hold.
func decryptQRC(hexText string) (string, error) {
	data, err := hex.DecodeString(strings.TrimSpace(hexText))
	if err != nil {
		return "", err
	}
	if len(data) < 8 {
		return "", errors.New("wordlyrics: qrc too short")
	}
	keys := qqKeys(qqKey, true)
	plain := qqTripleDES(data, keys)
	r, err := zlib.NewReader(bytes.NewReader(plain))
	if err != nil {
		return "", err
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(string(out), "\ufeff"), nil
}

var (
	qrcContent = regexp.MustCompile(`(?s)<Lyric_1 LyricType="1" LyricContent="(.*?)"/>`)
	qrcLine    = regexp.MustCompile(`^\[(\d+),(\d+)\](.*)$`)
	qrcWord    = regexp.MustCompile(`\((\d+),(\d+)\)`)
)

// parseQRC reads the lines and words out of QQ's QRC text. Each word stands
// before the (start,duration) that ends it.
func parseQRC(text string) []lyrics.Line {
	m := qrcContent.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	content := html.UnescapeString(m[1])
	var out []lyrics.Line
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, "\r")
		g := qrcLine.FindStringSubmatch(line)
		if g == nil {
			continue
		}
		body := g[3]

		var words []lyrics.Word
		prev := 0
		for _, w := range qrcWord.FindAllStringSubmatchIndex(body, -1) {
			word := body[prev:w[0]]
			at, _ := strconv.Atoi(body[w[2]:w[3]])
			d, _ := strconv.Atoi(body[w[4]:w[5]])
			if word != "" {
				words = append(words, lyrics.Word{
					Text:  word,
					Start: time.Duration(at) * time.Millisecond,
					End:   time.Duration(at+d) * time.Millisecond,
				})
			}
			prev = w[1]
		}
		if len(words) == 0 {
			continue
		}
		out = append(out, lyrics.Line{
			// The words decide where the line starts and ends: they are what
			// the player scrolls by.
			At:    words[0].Start,
			End:   words[len(words)-1].End,
			Text:  joinWords(words),
			Words: words,
			Timed: true,
		})
	}
	return out
}

// joinWords runs the words of a line together.
func joinWords(words []lyrics.Word) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString(w.Text)
	}
	return b.String()
}

// qq looks for s in QQ Music, whose catalogue carries word timing for most
// songs a listener in China plays. A search that turns up nothing is asked
// again by name alone: files carry subtitles the search does not.
func qq(ctx context.Context, s Song) ([]lyrics.Line, error) {
	if s.Name == "" {
		return nil, errNone
	}
	queries := make([]string, 0, 3)
	if len(s.Artists) > 0 {
		queries = append(queries, strings.Join(s.Artists, " ")+" "+s.Name)
	}
	queries = append(queries, s.Name)
	if bare := withoutBrackets(s.Name); bare != s.Name && bare != "" {
		queries = append(queries, bare)
	}

	var failed error
	for _, q := range queries {
		songs, err := qqSearch(ctx, q)
		if err != nil {
			failed = err
			continue
		}
		pick := pickQQ(songs, s)
		if pick == nil {
			continue
		}
		encrypted, err := qqEncrypted(ctx, pick.Mid)
		if err != nil {
			failed = err
			continue
		}
		if encrypted == "" {
			continue
		}
		plain, err := decryptQRC(encrypted)
		if err != nil {
			failed = err
			continue
		}
		if lines := parseQRC(plain); len(lines) > 0 {
			return lines, nil
		}
	}
	return nil, failed
}

// withoutBrackets is name without the subtitle a file adds to it.
func withoutBrackets(name string) string {
	if i := strings.IndexAny(name, "(（[【"); i > 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

// pickQQ is the first of songs that is s. QQ credits songs under the names
// China knows them by, 中岛美雪 for 中島みゆき, so the artist is what a pick
// tries first and what it does without: the name and the length have to
// agree either way.
func pickQQ(songs []qqSong, s Song) *qqSong {
	named := func(loose bool) []*qqSong {
		name := Fold(s.Name)
		var out []*qqSong
		for i := range songs {
			c := &songs[i]
			if c.Mid == "" {
				continue
			}
			other := Fold(c.Name)
			if other == name || (loose && oneRuneApart(name, other)) {
				out = append(out, c)
			}
		}
		return out
	}
	alike := func(c *qqSong) bool {
		if s.Length == 0 || c.Interval == 0 {
			return true
		}
		gap := time.Duration(c.Interval)*time.Second - s.Length
		return gap >= -qqTolerance && gap <= qqTolerance
	}

	cands := append(named(false), named(true)...)
	for _, c := range cands {
		withArtist := strings.Join(c.artists(), "/")
		if alike(c) && len(s.Artists) > 0 &&
			(SharesArtist(withArtist, s.Artists) || artistAlike(withArtist, s.Artists)) {
			return c
		}
	}
	for _, c := range cands {
		if alike(c) {
			return c
		}
	}
	return nil
}

// oneRuneApart says a and b are of a length and differ in a single rune,
// which is how the same title crosses the strait.
func oneRuneApart(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	if len(ra) != len(rb) || len(ra) < 2 {
		return false
	}
	diff := 0
	for i := range ra {
		if ra[i] != rb[i] {
			diff++
			if diff > 1 {
				return false
			}
		}
	}
	return diff == 1
}

// artistAlike reports whether singer names one of artists as closely as the
// strait names them: QQ writes 後ろから這いより隊G where a file has
// 后ろから这いより队G, and SharesArtist would take them for two.
func artistAlike(singer string, artists []string) bool {
	got := []rune(Fold(singer))
	if len(got) < 2 {
		return false
	}
	for _, a := range artists {
		want := []rune(Fold(a))
		if len(want) < 2 {
			continue
		}
		left := make(map[rune]int, len(got))
		for _, r := range got {
			left[r]++
		}
		common := 0
		for _, r := range want {
			if left[r] > 0 {
				left[r]--
				common++
			}
		}
		longer := len(got)
		if len(want) > longer {
			longer = len(want)
		}
		if float64(common)/float64(longer) >= 0.6 {
			return true
		}
	}
	return false
}
