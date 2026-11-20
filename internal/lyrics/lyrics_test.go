package lyrics

import (
	"testing"
	"time"
)

func TestParseLRC(t *testing.T) {
	lines := mergeLyrics("[00:01.000] 作词 : 某人\n[00:30.54]故事的小黄花\n[00:34.165][01:10.1]从出生那年就飘着\n[00:40.00]\n",
		"[00:30.54]The little yellow flower\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines: %+v", len(lines), lines)
	}
	if lines[1].At != 30540*time.Millisecond || lines[1].Translation != "The little yellow flower" {
		t.Errorf("line 1: %+v", lines[1])
	}
	if lines[3].At != 70100*time.Millisecond || lines[3].Text != "从出生那年就飘着" {
		t.Errorf("line 3: %+v", lines[3])
	}
	if i := Index(lines, 33*time.Second); i != 1 {
		t.Errorf("index at 33s = %d", i)
	}
	if i := Index(lines, 0); i != -1 {
		t.Errorf("index at 0 = %d", i)
	}
}

func TestParseYRC(t *testing.T) {
	yrc := `{"t":0,"c":[{"tx":"作词: "},{"tx":"某人"}]}
[13010,4780](13010,660,0)从(13670,670,0)不(14340,30,0) (14370,480,0)主
[17790,1770](17790,170,0)I(17960,450,0) (18410,290,0)love`
	lines := Build("", "[00:13.00]Never\n[00:17.80]我爱", yrc)
	if len(lines) != 2 || !lines[0].Timed {
		t.Fatalf("lines %+v", lines)
	}
	l := lines[0]
	if l.Text != "从不 主" || l.At != 13010*time.Millisecond || l.End != 17790*time.Millisecond {
		t.Errorf("line %q %v-%v", l.Text, l.At, l.End)
	}
	if len(l.Words) != 3 || l.Words[1].Text != "不 " || l.Words[2].Start != 14370*time.Millisecond {
		t.Errorf("words %+v", l.Words)
	}
	if l.Translation != "Never" || lines[1].Translation != "我爱" {
		t.Errorf("translations %q %q", l.Translation, lines[1].Translation)
	}
}

func TestSpreadWords(t *testing.T) {
	w := SpreadWords("我爱 you all", 0, 10*time.Second)
	got := []string{}
	for _, x := range w {
		got = append(got, x.Text)
	}
	if len(w) != 4 || got[0] != "我" || got[1] != "爱" || got[2] != "you " || got[3] != "all" {
		t.Fatalf("words %q", got)
	}
	if w[0].Start != 0 || w[3].End > 10*time.Second || w[1].Start != w[0].End {
		t.Errorf("times %+v", w)
	}
}

func TestQuiets(t *testing.T) {
	s := time.Second
	lines := []Line{
		{At: 0, End: 2 * s, Text: "作词 : 林夕"},
		{At: 2 * s, End: 4 * s, Text: "作曲：陈小霞"},
		{At: 20 * s, End: 25 * s, Text: "一"},
		{At: 26 * s, End: 30 * s, Text: "二"},
		{At: 45 * s, End: 50 * s, Text: "三"},
	}
	got := Quiets(lines, 60*s, 8*s)
	want := []Quiet{{0, 20 * s}, {30 * s, 45 * s}, {50 * s, 60 * s}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quiet %d: %v, want %v", i, got[i], want[i])
		}
	}
	if q := Quiets(nil, 60*s, 8*s); len(q) != 0 {
		t.Errorf("a song without lyrics: %v", q)
	}
}

func TestIsCredit(t *testing.T) {
	for text, want := range map[string]bool{
		"作词 : 林夕":            true,
		"作曲：陈小霞":             true,
		"编曲: 藤田淳平(Elements)": true,
		"Producer: 周杰伦":      true,
		"我说：你好":              false,
		"时间：是一把刀":            false,
		"悴んだ心 ふるえる眼差し":       false,
		"词：方文山":              true,
		"曲终人散：空余叹":           false,
	} {
		if IsCredit(text) != want {
			t.Errorf("%q: %v", text, !want)
		}
	}
}

func TestFromText(t *testing.T) {
	timed := FromText("[00:01.00]一\r\n[00:02.00]二")
	if len(timed) != 2 || IsPlain(timed) || timed[1].Text != "二" {
		t.Fatalf("timed: %+v", timed)
	}
	plain := FromText("一\n\n\n二\n")
	if !IsPlain(plain) || len(plain) != 3 || plain[1].Text != "" || plain[2].Text != "二" {
		t.Fatalf("plain: %+v", plain)
	}
	if Index(plain, 3*time.Minute) != -1 {
		t.Error("a plain line is lit")
	}
	if Quiets(plain, 3*time.Minute, time.Second) != nil {
		t.Error("plain lyrics have quiets")
	}
}

// Enhanced LRC markers time the words without appearing in lyrics or translations.
func TestEnhancedLRC(t *testing.T) {
	lines := FromText("[03:08.359]<03:08.359>不管<03:11.029>有多少人<03:15.030>\n[03:16.000]<03:16.000>次\n[03:18.000]終")
	if len(lines) != 3 || lines[0].Text != "不管有多少人" || !lines[0].Timed ||
		len(lines[0].Words) != 2 || lines[0].Words[1].Start != 191029*time.Millisecond ||
		lines[0].End != 195030*time.Millisecond || lines[1].Words[0].End != 198*time.Second ||
		lines[2].Timed {
		t.Fatalf("incorrect enhanced lyrics: %+v", lines)
	}
	translated := Build("[00:02]歌", "[00:02]<00:02>译文<00:03>", "")
	if translated[0].Translation != "译文" {
		t.Fatalf("incorrect translation: %+v", translated)
	}
}

// A bilingual enhanced LRC keeps the original active at a shared timestamp.
func TestBilingualEnhancedLRC(t *testing.T) {
	lines := FromText("[01:16.149]<01:16.149>夕<01:16.349>陽<01:19.779>\n[01:16.149]<01:16.149>夕阳染红的街道<01:19.770>\n[01:21.269]次")
	if len(lines) != 2 || Index(lines, 77*time.Second) != 0 ||
		lines[0].Text != "夕陽" || lines[0].Translation != "夕阳染红的街道" ||
		!lines[0].Timed || len(lines[0].Words) != 2 ||
		lines[0].Words[1].Start != 76349*time.Millisecond {
		t.Fatalf("incorrect bilingual lyrics: %+v", lines)
	}
}
