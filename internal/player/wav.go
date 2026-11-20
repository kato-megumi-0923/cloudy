package player

import (
	"io"
	"sync"

	"netease-music-mygo/internal/wav"
)

type wavReader struct{ dl *download }

func (r wavReader) ReadAt(b []byte, offset int64) (int, error) {
	return io.ReadFull(&downloadReader{d: r.dl, pos: offset}, b)
}

type wavStream struct {
	dl   *download
	info *wav.File
	pos  int64
	buf  []byte
	spec *spectrum
	drop func()
	once sync.Once
}

func openWAVStream(dl *download) (*wavStream, error) {
	info, err := wav.Open(wavReader{dl}, dl.size)
	if err != nil {
		return nil, err
	}
	return &wavStream{dl: dl, info: info, spec: newSpectrum(int(info.Rate), info.Frames()), drop: dl.Close}, nil
}

func (s *wavStream) SampleRate() int                { return int(s.info.Rate) }
func (s *wavStream) Channels() int                  { return 2 }
func (s *wavStream) Len() int64                     { return s.info.Frames() }
func (s *wavStream) SetPosition(frame int64) error  { s.pos = max(0, min(s.Len(), frame)); return nil }
func (s *wavStream) source() (*download, *spectrum) { return s.dl, s.spec }
func (s *wavStream) Close() error                   { s.once.Do(s.drop); return nil }

func (s *wavStream) Read(dst []float32) (int, error) {
	frames := min(int64(len(dst)/2), s.Len()-s.pos, 4096)
	if frames == 0 {
		if s.pos == s.Len() {
			return 0, io.EOF
		}
		return 0, nil
	}
	bytes := int(frames) * int(s.info.Align)
	if cap(s.buf) < bytes {
		s.buf = make([]byte, bytes)
	}
	buf := s.buf[:bytes]
	n, err := (wavReader{s.dl}).ReadAt(buf, s.info.Data+s.pos*int64(s.info.Align))
	frames = int64(n / int(s.info.Align))
	width := int(s.info.Bits / 8)
	for i := 0; i < int(frames); i++ {
		frame := buf[i*int(s.info.Align):]
		left := s.info.Sample(frame)
		right := left
		if s.info.Channels == 2 {
			right = s.info.Sample(frame[width:])
		}
		dst[i*2], dst[i*2+1] = left, right
	}
	s.spec.feed(s.pos, dst[:frames*2])
	s.pos += frames
	return int(frames) * 2, err
}
