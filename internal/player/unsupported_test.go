package player

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// AAC and the other formats the player cannot decode are told apart from
// MP3 at once, and the song is skipped, not fetched again and again.
func TestUnsupportedFormatSkipped(t *testing.T) {
	m4a := append([]byte{0, 0, 0, 0x20}, []byte("ftypM4A \x00\x00\x02\x00")...)
	adts := []byte{0xff, 0xf1, 0x50, 0x80, 0, 0x1f, 0xfc, 0, 0, 0, 0, 0}
	path := filepath.Join(t.TempDir(), "song")
	for name, head := range map[string][]byte{"m4a": m4a, "adts": adts} {
		data := append(append([]byte(nil), head...), make([]byte, 64<<10)...)
		os.WriteFile(path, data, 0o644)
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = openStream(opened(f, int64(len(data))), 1000)
		f.Close()
		var format *UnsupportedError
		if !errors.As(err, &format) {
			t.Fatalf("%s: %v, want UnsupportedError", name, err)
		}
		if !unplayable(err) {
			t.Errorf("%s: not skipped", name)
		}
		if _, again := retryWait(err, 0); again {
			t.Errorf("%s: fetched again", name)
		}
	}
	for _, mp3 := range [][]byte{{0xff, 0xfb, 0x90, 0x64}, {0xff, 0xf3, 0x90, 0x64}, {0xff, 0xe3, 0x90, 0x64}, []byte("ID3\x04")} {
		if f := otherFormat(mp3); f != "" {
			t.Errorf("% x taken for %s", mp3, f)
		}
	}
}
