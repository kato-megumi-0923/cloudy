// Package wav reads RIFF/WAVE headers and uncompressed audio samples.
package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

type Chunk struct {
	ID           string
	Offset, Size int64
}

type File struct {
	Format, Channels, Bits uint16
	Rate                   uint32
	Align                  uint16
	Data, Size             int64
	Chunks                 []Chunk
}

func Open(r io.ReaderAt, size int64) (*File, error) {
	header := make([]byte, 12)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, err
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return nil, errors.New("wav: expected RIFF/WAVE")
	}
	end := int64(binary.LittleEndian.Uint32(header[4:])) + 8
	if end < 12 || size > 0 && end > size {
		return nil, io.ErrUnexpectedEOF
	}
	f := &File{}
	var haveFormat, haveData bool
	for offset := int64(12); offset < end; {
		if end-offset < 8 {
			return nil, io.ErrUnexpectedEOF
		}
		h := make([]byte, 8)
		if _, err := r.ReadAt(h, offset); err != nil {
			return nil, err
		}
		chunk := Chunk{string(h[:4]), offset + 8, int64(binary.LittleEndian.Uint32(h[4:]))}
		next := chunk.Offset + chunk.Size + chunk.Size%2
		if next > end {
			return nil, io.ErrUnexpectedEOF
		}
		f.Chunks = append(f.Chunks, chunk)
		switch chunk.ID {
		case "fmt ":
			if haveFormat || chunk.Size < 16 {
				return nil, errors.New("wav: invalid format chunk")
			}
			b := make([]byte, min(chunk.Size, 40))
			if _, err := r.ReadAt(b, chunk.Offset); err != nil {
				return nil, err
			}
			f.Format, f.Channels = binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint16(b[2:])
			f.Rate, f.Align, f.Bits = binary.LittleEndian.Uint32(b[4:]), binary.LittleEndian.Uint16(b[12:]), binary.LittleEndian.Uint16(b[14:])
			if f.Format == 0xfffe {
				if len(b) < 40 || binary.LittleEndian.Uint16(b[16:]) < 22 || int64(binary.LittleEndian.Uint16(b[16:]))+18 > chunk.Size {
					return nil, errors.New("wav: incomplete extensible format")
				}
				valid := binary.LittleEndian.Uint16(b[18:])
				if valid == 0 || valid > f.Bits {
					return nil, errors.New("wav: invalid valid bits")
				}
				if string(b[26:40]) != "\x00\x00\x00\x00\x10\x00\x80\x00\x00\xaa\x00\x38\x9b\x71" {
					return nil, errors.New("wav: unsupported extensible subformat")
				}
				f.Format = binary.LittleEndian.Uint16(b[24:])
			}
			haveFormat = true
		case "data":
			if haveData {
				return nil, errors.New("wav: multiple data chunks")
			}
			f.Data, f.Size, haveData = chunk.Offset, chunk.Size, true
		}
		offset = next
	}
	if !haveFormat || !haveData || f.Rate == 0 || f.Rate > 768000 || f.Channels < 1 || f.Channels > 2 {
		return nil, errors.New("wav: expected mono or stereo audio with a valid sample rate")
	}
	if !(f.Format == 1 && (f.Bits == 8 || f.Bits == 16 || f.Bits == 24 || f.Bits == 32) ||
		f.Format == 3 && (f.Bits == 32 || f.Bits == 64)) {
		return nil, fmt.Errorf("wav: unsupported encoding %d/%d bits", f.Format, f.Bits)
	}
	if f.Align != f.Channels*(f.Bits/8) || f.Size%int64(f.Align) != 0 {
		return nil, errors.New("wav: invalid block alignment")
	}
	return f, nil
}

func (f *File) Frames() int64 { return f.Size / int64(f.Align) }

// Sample reads one little-endian sample. PCM valid bits in extensible WAVE
// are left aligned, so the container width also gives the correct scale.
func (f *File) Sample(b []byte) float32 {
	if f.Format == 3 {
		var value float64
		if f.Bits == 32 {
			value = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		} else {
			value = math.Float64frombits(binary.LittleEndian.Uint64(b))
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		return float32(max(-1, min(1, value)))
	}
	switch f.Bits {
	case 8:
		return (float32(b[0]) - 128) / 128
	case 16:
		return float32(int16(binary.LittleEndian.Uint16(b))) / 32768
	case 24:
		n := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		return float32(n<<8>>8) / 8388608
	default:
		return float32(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	}
}
