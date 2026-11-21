package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// dsdTone is a second of a 1 kHz tone at amplitude 0.5, as DSD64: what a
// second-order delta-sigma modulator makes of it, one bit a sample, for
// each of two channels.
func dsdTone() [][]bool {
	const rate = 2822400
	out := make([][]bool, 2)
	for c := range out {
		var i1, i2 float64
		bits := make([]bool, rate)
		for n := range bits {
			x := 0.5 * math.Sin(2*math.Pi*1000*float64(n)/rate)
			y := -1.0
			if i2 >= 0 {
				y = 1
			}
			i1 += x - y
			i2 += i1 - y
			bits[n] = y > 0
		}
		out[c] = bits
	}
	return out
}

func packBits(bits []bool, lsbFirst bool) []byte {
	b := make([]byte, (len(bits)+7)/8)
	for i, on := range bits {
		if !on {
			continue
		}
		if lsbFirst {
			b[i/8] |= 1 << (i % 8)
		} else {
			b[i/8] |= 0x80 >> (i % 8)
		}
	}
	return b
}

func dsfFile(ch [][]bool) []byte {
	const block = 4096
	data := [][]byte{packBits(ch[0], true), packBits(ch[1], true)}
	per := (len(data[0]) + block - 1) / block * block
	var body bytes.Buffer
	for g := 0; g < per; g += block {
		for _, d := range data {
			chunk := make([]byte, block)
			if g < len(d) {
				copy(chunk, d[g:])
			}
			body.Write(chunk)
		}
	}
	le := binary.LittleEndian
	var f bytes.Buffer
	f.WriteString("DSD ")
	binary.Write(&f, le, uint64(28))
	binary.Write(&f, le, uint64(28+52+12+body.Len()))
	binary.Write(&f, le, uint64(0))
	f.WriteString("fmt ")
	binary.Write(&f, le, uint64(52))
	for _, v := range []uint32{1, 0, 2, 2, 2822400, 1} {
		binary.Write(&f, le, v)
	}
	binary.Write(&f, le, uint64(len(ch[0])))
	binary.Write(&f, le, uint32(block))
	binary.Write(&f, le, uint32(0))
	f.WriteString("data")
	binary.Write(&f, le, uint64(12+body.Len()))
	f.Write(body.Bytes())
	return f.Bytes()
}

func dffFile(ch [][]bool) []byte {
	be := binary.BigEndian
	l, r := packBits(ch[0], false), packBits(ch[1], false)
	sound := make([]byte, 0, 2*len(l))
	for i := range l {
		sound = append(sound, l[i], r[i])
	}
	var prop bytes.Buffer
	prop.WriteString("SND ")
	prop.WriteString("FS  ")
	binary.Write(&prop, be, uint64(4))
	binary.Write(&prop, be, uint32(2822400))
	prop.WriteString("CHNL")
	binary.Write(&prop, be, uint64(10))
	binary.Write(&prop, be, uint16(2))
	prop.WriteString("SLFTSRGT")
	prop.WriteString("CMPR")
	binary.Write(&prop, be, uint64(4))
	prop.WriteString("DSD ")
	var body bytes.Buffer
	body.WriteString("DSD ")
	body.WriteString("PROP")
	binary.Write(&body, be, uint64(prop.Len()))
	body.Write(prop.Bytes())
	body.WriteString("DSD ")
	binary.Write(&body, be, uint64(len(sound)))
	body.Write(sound)
	var f bytes.Buffer
	f.WriteString("FRM8")
	binary.Write(&f, be, uint64(body.Len()))
	f.Write(body.Bytes())
	return f.Bytes()
}

func decodeTone(t *testing.T, s stream) (frames, crossings int, power float64) {
	t.Helper()
	buf := make([]float32, 4096)
	prev := float32(0)
	for {
		n, err := s.Read(buf)
		for i := 0; i+1 < n; i += 2 {
			v := buf[i]
			if (v >= 0) != (prev >= 0) {
				crossings++
			}
			prev = v
			power += float64(v * v)
			frames++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if frames > 0 {
		power /= float64(frames)
	}
	return
}

func TestSharedDSDLeavesPlaybackOpen(t *testing.T) {
	tone := dsdTone()
	for name, b := range map[string][]byte{"DSF": dsfFile(tone), "DFF": dffFile(tone)} {
		t.Run(name, func(t *testing.T) { checkSharedPlayback(t, b, 1000) })
	}
}

// A separate decoder for loudness (or transcription) must not close the
// playing stream's download or try to close its lightweight view as a root.
func checkSharedPlayback(t *testing.T, b []byte, durationMs int64) {
	t.Helper()
	dl := downloaded(b)
	playing, err := openStream(dl, durationMs)
	if err != nil {
		t.Fatal(err)
	}
	defer playing.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shared, err := openSharedContext(ctx, dl, durationMs)
	if err != nil {
		t.Fatal(err)
	}
	if _, sp := shared.source(); sp != nil {
		t.Error("background decoder measured playback spectrum")
	}
	if n, _, _ := decodeTone(t, shared); n == 0 {
		t.Error("background decoder returned no audio")
	}
	shared.Close()
	shared.Close()
	cancel()
	if n, _, _ := decodeTone(t, playing); n == 0 {
		t.Error("playback stopped with the background decoder")
	}
}

// DSD, in a DSF or a DSDIFF, plays as 88.2 kHz PCM: the tone at its pitch
// and about its loudness, a second of it, from the start or from half in.
func TestDSDDecodes(t *testing.T) {
	tone := dsdTone()
	for name, file := range map[string][]byte{"tone.dsf": dsfFile(tone), "tone.dff": dffFile(tone)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			os.WriteFile(path, file, 0o644)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			s, err := openStream(opened(f, int64(len(file))), 1000)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if s.SampleRate() != 88200 || s.Len() != 88200 {
				t.Fatalf("rate %d, length %d", s.SampleRate(), s.Len())
			}
			frames, crossings, power := decodeTone(t, s)
			if frames != 88200 {
				t.Errorf("decoded %d frames", frames)
			}
			if hz := float64(crossings) / 2 / (float64(frames) / 88200); math.Abs(hz-1000) > 20 {
				t.Errorf("tone at %.0f Hz", hz)
			}
			if rms := math.Sqrt(power); rms < 0.3 || rms > 0.4 {
				t.Errorf("rms %.3f, want about 0.354", rms)
			}
			if err := s.SetPosition(44100); err != nil {
				t.Fatal(err)
			}
			if rest, _, _ := decodeTone(t, s); rest != 44100 {
				t.Errorf("from half in, %d frames", rest)
			}
		})
	}
}
