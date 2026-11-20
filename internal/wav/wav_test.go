package wav

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// WAV samples decode at their declared depth, including extensible PCM.
func TestSamplesAndFormats(t *testing.T) {
	for _, test := range []struct {
		name         string
		format, bits uint16
		sample       []byte
		value        float32
	}{
		{"pcm8", 1, 8, []byte{192}, .5},
		{"pcm16", 1, 16, []byte{0, 0x40}, .5},
		{"pcm24", 1, 24, []byte{0, 0, 0xc0}, -.5},
		{"pcm32", 1, 32, []byte{0, 0, 0, 0x40}, .5},
		{"float32", 3, 32, []byte{0, 0, 0, 0x3f}, .5},
		{"float64", 3, 64, []byte{0, 0, 0, 0, 0, 0, 0xe0, 0xbf}, -.5},
		{"extensible24", 0xfffe, 24, []byte{0, 0, 0x40}, .5},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := fixture(test.format, test.bits, test.sample)
			f, err := Open(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatal(err)
			}
			if f.Frames() != 1 || math.Abs(float64(f.Sample(test.sample)-test.value)) > 1e-6 {
				t.Fatalf("incorrect sample: %+v", f)
			}
		})
	}
}

func fixture(format, bits uint16, sample []byte) []byte {
	var body bytes.Buffer
	body.WriteString("WAVEfmt ")
	size := uint32(16)
	if format == 0xfffe {
		size = 40
	}
	binary.Write(&body, binary.LittleEndian, size)
	for _, n := range []uint16{format, 1} {
		binary.Write(&body, binary.LittleEndian, n)
	}
	binary.Write(&body, binary.LittleEndian, uint32(8000))
	binary.Write(&body, binary.LittleEndian, uint32(8000)*uint32(bits/8))
	for _, n := range []uint16{bits / 8, bits} {
		binary.Write(&body, binary.LittleEndian, n)
	}
	if format == 0xfffe {
		for _, n := range []uint16{22, bits} {
			binary.Write(&body, binary.LittleEndian, n)
		}
		binary.Write(&body, binary.LittleEndian, uint32(4))
		body.Write([]byte{1, 0, 0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71})
	}
	body.WriteString("JUNK")
	binary.Write(&body, binary.LittleEndian, uint32(1))
	body.Write([]byte{42, 0})
	body.WriteString("data")
	binary.Write(&body, binary.LittleEndian, uint32(len(sample)))
	body.Write(sample)
	if len(sample)%2 != 0 {
		body.WriteByte(0)
	}
	var file bytes.Buffer
	file.WriteString("RIFF")
	binary.Write(&file, binary.LittleEndian, uint32(body.Len()))
	file.Write(body.Bytes())
	return file.Bytes()
}

// Malformed RIFF sizes and compressed WAV must fail before decoding.
func TestRejectInvalidWAV(t *testing.T) {
	valid := fixture(1, 16, []byte{0, 0})
	compressed := fixture(6, 16, []byte{0, 0})
	unaligned := fixture(1, 16, []byte{0})
	for _, b := range [][]byte{valid[:9], valid[:len(valid)-1], compressed, unaligned} {
		if _, err := Open(bytes.NewReader(b), int64(len(b))); err == nil {
			t.Fatal("invalid WAV accepted")
		}
	}
}
