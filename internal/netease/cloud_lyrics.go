package netease

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"netease-music-mygo/internal/lyrics"
)

// A song of the cloud drive that NetEase matched to none of its own has no
// lyrics in the catalog: the song lyric APIs answer it with no lines, or
// with the one line "暂无歌词", which Cloudy showed as its lyrics (a viewer,
// 1.3.2). Its lyrics are those the drive keeps for the listener: what the
// uploaded file carried, or what they wrote in the official client. Only
// the cloud lyric API serves them, and only to their owner.

// placeholderLyrics reports whether lrc says only that there are none.
func placeholderLyrics(lrc string) bool {
	text := ""
	for _, line := range strings.Split(lrc, "\n") {
		line = strings.TrimSpace(line)
		for strings.HasPrefix(line, "[") {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				break
			}
			line = strings.TrimSpace(line[end+1:])
		}
		if line == "" {
			continue
		}
		if text != "" {
			return false
		}
		text = line
	}
	return text == "" || text == "暂无歌词"
}

// cloudUser is the signed-in listener's ID, asked once a session: the cloud
// lyric API wants it.
var cloudUser struct {
	sync.Mutex
	session string // the MUSIC_U it was asked with
	id      int64
}

// cloudUserID returns the signed-in listener's ID, or 0 when no one is.
func cloudUserID() int64 {
	session := ""
	for _, c := range httpClient.Jar.Cookies(siteURL) {
		if c.Name == "MUSIC_U" {
			session = c.Value
		}
	}
	if session == "" {
		return 0
	}
	cloudUser.Lock()
	defer cloudUser.Unlock()
	if cloudUser.session == session && cloudUser.id > 0 {
		return cloudUser.id
	}
	me, err := accountOf()
	if err != nil || me == nil {
		return 0
	}
	cloudUser.session, cloudUser.id = session, me.UserID
	return me.UserID
}

// accountOf is Account; tests replace it.
var accountOf = Account

// cloudLyricsOf is CloudLyrics; tests replace it.
var cloudLyricsOf = CloudLyrics

// CloudLyrics returns the lyrics the listener's cloud drive keeps for song
// id: line-timed, or word-timed as YRC where the drive has them as KRC.
// Both are empty for a song not in their drive, or with no lyrics there.
func CloudLyrics(id int64) (lrc, yrc string, err error) {
	uid := cloudUserID()
	if uid == 0 {
		return "", "", nil
	}
	var res struct {
		Code int    `json:"code"`
		Lrc  string `json:"lrc"`
		Krc  string `json:"krc"`
	}
	data := map[string]any{"userId": uid, "songId": id, "lv": -1, "kv": -1}
	if err := eapiPost("/api/cloud/lyric/get", data, &res); err != nil {
		return "", "", err
	}
	if res.Code != 0 && res.Code != 200 {
		return "", "", fmt.Errorf("netease: cloud lyrics: code %d", res.Code)
	}
	if !placeholderLyrics(res.Lrc) {
		lrc = res.Lrc
	}
	return lrc, krcToYRC(res.Krc), nil
}

var (
	krcLineRe = regexp.MustCompile(`^\[(\d+),(\d+)\](.*)$`)
	krcWordRe = regexp.MustCompile(`<(\d+),(\d+),\d+>([^<]*)`)
)

// krcToYRC turns KRC, plain or as Kugou sends it, into NetEase's YRC: the
// same lines, each word's start counted from the song's start, not the
// line's.
func krcToYRC(krc string) string {
	krc = strings.TrimSpace(krc)
	if krc == "" {
		return ""
	}
	if !strings.HasPrefix(krc, "[") {
		text, err := lyrics.DecodeKRC(krc)
		if err != nil {
			return ""
		}
		krc = text
	}
	var b strings.Builder
	for _, raw := range strings.Split(krc, "\n") {
		m := krcLineRe.FindStringSubmatch(strings.TrimSpace(raw))
		if m == nil {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		words := krcWordRe.FindAllStringSubmatch(m[3], -1)
		if len(words) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[%s,%s]", m[1], m[2])
		for _, w := range words {
			off, _ := strconv.Atoi(w[1])
			fmt.Fprintf(&b, "(%d,%s,0)%s", start+off, w[2], w[3])
		}
		b.WriteByte('\n')
	}
	return b.String()
}
