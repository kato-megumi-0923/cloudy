package player

import (
	"io"
	"os"
	"testing"
)

// WAV playback expands mono to stereo and seeks to the exact PCM frame.
func TestWAVReadAndSeek(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.wav")
	if err != nil {
		t.Fatal(err)
	}
	dl := downloaded(b)
	s, err := openStream(dl, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.SampleRate() != 8000 || s.Len() != 8000 || s.Channels() != 2 {
		t.Fatalf("incorrect WAV format")
	}
	var decoded []float32
	buf := make([]float32, 513)
	for {
		n, err := s.Read(buf)
		decoded = append(decoded, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(decoded) != 16000 {
		t.Fatalf("decoded %d samples", len(decoded))
	}
	for i := 0; i < len(decoded); i += 2 {
		if decoded[i] != decoded[i+1] {
			t.Fatal("mono was not expanded")
		}
	}
	if err := s.SetPosition(1234); err != nil {
		t.Fatal(err)
	}
	n, err := s.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("read after seek: %d %v", n, err)
	}
	for i := 0; i < n; i++ {
		if buf[i] != decoded[1234*2+i] {
			t.Fatal("WAV seek was not sample accurate")
		}
	}
	if err := s.SetPosition(s.Len() + 100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(buf); err != io.EOF {
		t.Fatalf("end seek: %v", err)
	}
}
