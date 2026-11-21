package lyrics

import (
	"strings"
	"testing"
	"time"
)

// Enhanced LRC keeps the words' own times and pairs each translation with
// its line when read again.
func TestToLRCWordsAndTranslation(t *testing.T) {
	lines := []Line{{
		At: 76*time.Second + 149*time.Millisecond, End: 79*time.Second + 779*time.Millisecond,
		Text: "夕陽", Translation: "夕阳染红的街道", Timed: true,
		Words: []Word{
			{Text: "夕", Start: 76*time.Second + 149*time.Millisecond, End: 76*time.Second + 349*time.Millisecond},
			{Text: "陽", Start: 76*time.Second + 349*time.Millisecond, End: 79*time.Second + 779*time.Millisecond},
		},
	}}
	want := "[01:16.149]<01:16.149>夕<01:16.349>陽<01:19.779>\r\n" +
		"[01:16.149]<01:16.149>夕阳染红的街道<01:19.779>\r\n"
	got := ToLRC(lines)
	if got != want {
		t.Fatalf("LRC = %q, want %q", got, want)
	}
	back := FromText(got)
	if len(back) != 1 {
		t.Fatalf("read back %d lines: %+v", len(back), back)
	}
	l := back[0]
	if l.Text != "夕陽" || l.Translation != "夕阳染红的街道" || !l.Timed || len(l.Words) != 2 {
		t.Fatalf("read back %+v", l)
	}
	if l.Words[1].Start != 76349*time.Millisecond || l.End != 79779*time.Millisecond {
		t.Fatalf("read back times %+v", l.Words)
	}
}

// Words spread along a line are not timed, and a file does not pretend
// they are: those lines stay plain.
func TestToLCRSpreadWordsStayPlain(t *testing.T) {
	got := ToLRC(Build("[00:01.00]一二三四\n[00:05.00]五六七八\n", "", ""))
	if strings.Contains(got, "<") {
		t.Fatalf("spread words written as timed: %q", got)
	}
	if back := FromText(got); len(back) != 2 || back[0].Timed {
		t.Fatalf("read back %+v", back)
	}
}

// A line without words keeps its time and its translation, and empty lines
// are left out.
func TestToLRCPlainLines(t *testing.T) {
	got := ToLRC([]Line{
		{At: time.Second, End: 2 * time.Second, Text: "一", Translation: "one"},
		{At: 2 * time.Second, Text: "  "},
		{At: 3 * time.Second, Text: "二"},
	})
	want := "[00:01.000]一\r\n[00:01.000]one\r\n[00:03.000]二\r\n"
	if got != want {
		t.Fatalf("LRC = %q, want %q", got, want)
	}
}
