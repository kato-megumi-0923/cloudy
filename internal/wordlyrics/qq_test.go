package wordlyrics

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

// The first blocks of a QRC QQ answered for ルージュ (Remaster), and what
// they decrypt to: the cipher is QQ's own, and standard DES turns these
// bytes into noise.
const (
	qqSampleHex = "368322F50971FDBA28127C2405AE29792CA101DD3249BDFD87C5936525E3A8198EE" +
		"CB1222602B4B68E8FA7D18C4642B24EC85453106440E12CC96BAAA4F9AB2F581A2D11655B2AFD9" +
		"CC058C3E51E79FDC9BB3C803950A657FA29634617AA8492"
	qqSamplePlain = `<?xml version="1.0" encoding="utf-8"?>` + "\n" + `<QrcInfos>` + "\n" + `<QrcHeadInfo SaveTime="277" Version="100"/>`
)

func TestQQTripleDES(t *testing.T) {
	data, err := hex.DecodeString(qqSampleHex)
	if err != nil {
		t.Fatal(err)
	}
	got := qqTripleDES(data, qqKeys(qqKey, true))
	// What QQ encrypts is a zlib stream, and its first bytes say so.
	if len(got) < 2 || got[0] != 0x78 || got[1] != 0x9c {
		t.Fatalf("不是 zlib 流的开头: % x", got[:8])
	}
	r, err := zlib.NewReader(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("inflate: %v", err)
	}
	defer r.Close()
	buf := make([]byte, len(qqSamplePlain))
	n, _ := io.ReadFull(r, buf)
	if !strings.HasPrefix(string(buf[:n]), qqSamplePlain[:n]) {
		t.Fatalf("明文开头不对:\n got %.120q\nwant %.120q", buf[:n], qqSamplePlain)
	}
}

// parseQRC reads QQ's lines and words: each word stands before the
// (start,duration) that ends it.
func TestParseQRC(t *testing.T) {
	const raw = `<?xml version="1.0" encoding="utf-8"?>
<QrcInfos>
<LyricInfo LyricCount="1">
<Lyric_1 LyricType="1" LyricContent="[ti:测试]
[100,1540]初(100,154)め(254,154)て(408,154)
[1640,270]BiliBili (1640,135)AV911093(1775,135)
"/>
</LyricInfo>
</QrcInfos>`
	lines := parseQRC(raw)
	if len(lines) != 2 {
		t.Fatalf("行数 = %d, want 2", len(lines))
	}
	first := lines[0]
	if !first.Timed {
		t.Fatal("没有标记为逐字")
	}
	if first.Text != "初めて" || len(first.Words) != 3 {
		t.Fatalf("第一行 = %q / %d 字", first.Text, len(first.Words))
	}
	if first.At.Milliseconds() != 100 || first.End.Milliseconds() != 562 {
		t.Fatalf("第一行时间 = %v..%v", first.At, first.End)
	}
	second := lines[1]
	if second.Text != "BiliBili AV911093" {
		t.Fatalf("第二行 = %q", second.Text)
	}
	if !strings.HasPrefix(raw, "<?xml") {
		t.Fatal("样本被改了")
	}
}

// pickQQ wants the same name and the same length, and a matching artist
// when QQ credits the song the way the file does.
func TestPickQQ(t *testing.T) {
	singer := func(name string) []struct {
		Name string `json:"name"`
	} {
		return []struct {
			Name string `json:"name"`
		}{{Name: name}}
	}
	songs := []qqSong{
		{Mid: "cover", Name: "恋は渾沌の隷也", Interval: 238, Singers: singer("ハロー、ハッピーワールド!")},
		{Mid: "right", Name: "恋は渾沌の隷也", Interval: 238, Singers: singer("後ろから這いより隊G")},
		{Mid: "short", Name: "恋は渾沌の隷也", Interval: 107, Singers: singer("後ろから這いより隊G")},
	}
	// 渾 vs 混 and the trad/simp artist: the length decides.
	got := pickQQ(songs, Song{Name: "恋は混沌の隷也", Artists: []string{"后ろから这いより队G"}, Length: 238 * 1000 * 1000 * 1000})
	if got == nil || got.Mid != "right" {
		t.Fatalf("选中 = %v", got)
	}
	// A same-name song of another length is another song.
	got = pickQQ(songs, Song{Name: "恋は渾沌の隷也", Length: 108 * 1000 * 1000 * 1000})
	if got == nil || got.Mid != "short" {
		t.Fatalf("按时长选中 = %v", got)
	}
	// Nothing of that name at all.
	if pickQQ(songs, Song{Name: "別の歌", Length: 238 * 1000 * 1000 * 1000}) != nil {
		t.Fatal("不该选中别的歌")
	}
}
