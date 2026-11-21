package player

import (
	"math"
	"os"
	"testing"
)

// AAC in an M4A and bare (ADTS), and ALAC in an M4A, play through Media
// Foundation: two seconds of a 440 Hz tone, at its rate, from the start
// and from a second in.
func TestMediaFoundationDecodes(t *testing.T) {
	for _, name := range []string{"sine.m4a", "sine.aac", "sine-alac.m4a"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			info, _ := f.Stat()
			s, err := openStream(opened(f, info.Size()), 2000)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if s.SampleRate() != 44100 {
				t.Fatalf("rate %d", s.SampleRate())
			}
			if n := s.Len(); n < 85000 || n > 92000 {
				t.Errorf("length %d frames, want about 88200", n)
			}
			frames, crossings, power := decodeTone(t, s)
			if frames < 80000 || frames > 95000 {
				t.Errorf("decoded %d frames", frames)
			}
			if hz := float64(crossings) / 2 / (float64(frames) / 44100); math.Abs(hz-440) > 15 {
				t.Errorf("tone at %.0f Hz", hz)
			}
			if power < 0.001 {
				t.Errorf("silent: power %f", power)
			}
			if err := s.SetPosition(44100); err != nil {
				t.Fatal(err)
			}
			rest, _, _ := decodeTone(t, s)
			if rest < 40000 || rest > 48000 {
				t.Errorf("from a second in, %d frames, want about 44100", rest)
			}
		})
	}
}

func TestSharedMediaFoundationLeavesPlaybackOpen(t *testing.T) {
	for _, name := range []string{"sine.m4a", "sine.aac", "sine-alac.m4a"} {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			checkSharedPlayback(t, b, 2000)
		})
	}
}
