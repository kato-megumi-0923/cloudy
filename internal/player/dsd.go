package player

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
)

// dsdStream decodes DSD, as DSF or DSDIFF (DFF) files hold it, into PCM
// for package audio (a viewer asked, 1.3.2): one bit per sample, at 64 or
// more times CD's rate, low-passed and decimated to 88.2 or 96 kHz. The
// filter is a windowed sinc, summed a byte (eight taps) at a time from
// tables, as DSD players do.
type dsdStream struct {
	dl       *download
	channels int
	outRate  int
	step     int   // bytes of each channel per frame out
	frames   int64 // frames out, in all
	lsbFirst bool  // DSF's bits run from the low one; DSDIFF's from the high
	// Where the sound is: DSF keeps it in blocks of block bytes for each
	// channel in turn, DSDIFF a byte of each channel in turn.
	start, length int64
	block         int
	dsf           bool           // in DSF's blocks
	dsfSkip       int            // bytes of the first block to drop, after a seek
	tables        [][256]float32 // for each byte of the window
	win           [][]byte       // each channel's bytes, the window's first
	r             *bufio.Reader
	at            int64 // the byte of each channel r reads next
	pos           int64 // the frame Read returns next
	pend          []float32
	spec          *spectrum
	drop          func()
	once          sync.Once
}

// dsdCutoff is where the filter starts cutting, well under the noise DSD
// shapes above the band, which would otherwise alias down into it.
const dsdCutoff = 24000

// dsdHeader is what a DSD file says of its sound.
type dsdHeader struct {
	rate, channels int
	samples        int64 // DSD samples of each channel
	start, length  int64
	block          int
	lsbFirst       bool
}

// readDSF reads a DSF's chunks: "DSD ", "fmt " and "data".
func readDSFHeader(dl *download) (dsdHeader, error) {
	if dl.waitFor(92) < 92 {
		return dsdHeader{}, io.ErrUnexpectedEOF
	}
	b := dl.peek(0, 92)
	le := binary.LittleEndian
	if string(b[:4]) != "DSD " || string(b[28:32]) != "fmt " {
		return dsdHeader{}, errors.New("dsd: not a DSF file")
	}
	fmtSize := int64(le.Uint64(b[32:40]))
	h := dsdHeader{
		channels: int(le.Uint32(b[52:56])),
		rate:     int(le.Uint32(b[56:60])),
		samples:  int64(le.Uint64(b[64:72])),
		block:    int(le.Uint32(b[72:76])),
		lsbFirst: le.Uint32(b[60:64]) == 1,
	}
	if le.Uint32(b[44:48]) != 0 {
		return dsdHeader{}, errors.New("dsd: not raw DSD")
	}
	data := 28 + fmtSize
	if dl.waitFor(data+12) < data+12 {
		return dsdHeader{}, io.ErrUnexpectedEOF
	}
	d := dl.peek(data, 12)
	if string(d[:4]) != "data" {
		return dsdHeader{}, errors.New("dsd: no data chunk")
	}
	h.start, h.length = data+12, int64(le.Uint64(d[4:12]))-12
	if h.block <= 0 {
		h.block = 4096
	}
	return h, nil
}

// readDFFHeader reads a DSDIFF's chunks: its properties, then its sound.
func readDFFHeader(dl *download) (dsdHeader, error) {
	be := binary.BigEndian
	if dl.waitFor(16) < 16 {
		return dsdHeader{}, io.ErrUnexpectedEOF
	}
	if b := dl.peek(0, 16); string(b[:4]) != "FRM8" || string(b[12:16]) != "DSD " {
		return dsdHeader{}, errors.New("dsd: not a DSDIFF file")
	}
	h := dsdHeader{channels: 2}
	for off := int64(16); ; {
		if dl.waitFor(off+12) < off+12 {
			return dsdHeader{}, io.ErrUnexpectedEOF
		}
		c := dl.peek(off, 12)
		id, n := string(c[:4]), int64(be.Uint64(c[4:12]))
		body := off + 12
		switch id {
		case "PROP":
			if n > 1<<20 || dl.waitFor(body+n) < body+n {
				return dsdHeader{}, io.ErrUnexpectedEOF
			}
			p := dl.peek(body, int(n))
			for i := 4; i+12 <= len(p); {
				sid, sn := string(p[i:i+4]), int(be.Uint64(p[i+4:i+12]))
				if i+12+sn > len(p) {
					break
				}
				v := p[i+12 : i+12+sn]
				switch sid {
				case "FS  ":
					if len(v) >= 4 {
						h.rate = int(be.Uint32(v))
					}
				case "CHNL":
					if len(v) >= 2 {
						h.channels = int(be.Uint16(v))
					}
				case "CMPR":
					if len(v) >= 4 && string(v[:4]) != "DSD " {
						return dsdHeader{}, errors.New("dsd: compressed (DST) DSDIFF")
					}
				}
				i += 12 + sn + sn&1
			}
		case "DSD ":
			h.start, h.length = body, n
			h.samples = n * 8 / int64(max(h.channels, 1))
			return h, nil
		}
		off = body + n + n&1
	}
}

func openDSDStream(dl *download) (*dsdStream, error) {
	var h dsdHeader
	var err error
	if dl.waitFor(4) >= 4 && string(dl.peek(0, 4)) == "FRM8" {
		h, err = readDFFHeader(dl)
	} else {
		h, err = readDSFHeader(dl)
	}
	dsf := err == nil && h.block > 0
	if err != nil {
		return nil, err
	}
	if h.channels <= 0 || h.channels > 8 || h.rate < 2822400/2 {
		return nil, errors.New("dsd: unexpected format")
	}
	// 88.2 kHz from the 44.1 family, 96 from the 48.
	out := 88200
	if h.rate%88200 != 0 {
		out = 96000
	}
	bits := h.rate / out
	if bits%8 != 0 || bits < 8 {
		return nil, errors.New("dsd: unexpected rate")
	}
	s := &dsdStream{
		dl: dl, channels: h.channels, outRate: out, step: bits / 8,
		frames: h.samples / int64(bits), lsbFirst: h.lsbFirst,
		start: h.start, length: h.length, block: h.block, dsf: dsf, drop: dl.Close,
	}
	s.tables = dsdTables(h.rate, 32*s.step, h.lsbFirst)
	s.spec = dl.spectrum(out, s.frames)
	s.seekBytes(0)
	return s, nil
}

// dsdTables are the filter's taps, eight to a byte, summed for each value
// the byte can have: n bytes of them.
func dsdTables(rate, n int, lsbFirst bool) [][256]float32 {
	taps := make([]float64, n*8)
	fc := float64(dsdCutoff) / float64(rate)
	sum := 0.0
	for i := range taps {
		x := float64(i) - float64(len(taps)-1)/2
		v := 2 * fc
		if x != 0 {
			v = math.Sin(2*math.Pi*fc*x) / (math.Pi * x)
		}
		// Blackman.
		w := 0.42 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(len(taps)-1)) + 0.08*math.Cos(4*math.Pi*float64(i)/float64(len(taps)-1))
		taps[i] = v * w
		sum += taps[i]
	}
	tables := make([][256]float32, n)
	for j := range tables {
		for b := 0; b < 256; b++ {
			v := 0.0
			for k := 0; k < 8; k++ {
				bit := b >> (7 - k) & 1
				if lsbFirst {
					bit = b >> k & 1
				}
				if bit == 1 {
					v += taps[j*8+k] / sum
				} else {
					v -= taps[j*8+k] / sum
				}
			}
			tables[j][b] = float32(v)
		}
	}
	return tables
}

func (s *dsdStream) SampleRate() int { return s.outRate }
func (s *dsdStream) Channels() int   { return 2 }
func (s *dsdStream) Len() int64      { return s.frames }

// seekBytes reads on from byte at of each channel, the window filled with
// silence before it: DSD's silence is ones and zeros by turns.
func (s *dsdStream) seekBytes(at int64) {
	n := len(s.tables)
	s.win = make([][]byte, s.channels)
	for c := range s.win {
		w := make([]byte, n-s.step, n*4)
		for i := range w {
			w[i] = 0x69 // 01101001: as many ones as zeros
		}
		s.win[c] = w
	}
	s.at = at
	off := s.start + at*int64(s.channels)
	if s.dsf {
		group := at / int64(s.block)
		off = s.start + group*int64(s.block)*int64(s.channels)
	}
	s.r = bufio.NewReaderSize(&downloadReader{d: s.dl, pos: off}, 64<<10)
	s.dsfSkip = 0
	if s.dsf {
		s.dsfSkip = int(at % int64(s.block))
	}
}

// fill reads more of each channel into its window: a block group of DSF,
// or a stretch of DSDIFF.
func (s *dsdStream) fill() error {
	total := s.length / int64(s.channels)
	if s.at >= total {
		return io.EOF
	}
	if s.dsf {
		group := make([]byte, s.block*s.channels)
		if _, err := io.ReadFull(s.r, group); err != nil {
			return io.EOF
		}
		from := s.dsfSkip
		s.dsfSkip = 0
		for c := range s.win {
			s.win[c] = append(s.win[c], group[c*s.block+from:(c+1)*s.block]...)
		}
		s.at += int64(s.block - from)
		return nil
	}
	chunk := make([]byte, 4096*s.channels)
	n, _ := io.ReadFull(s.r, chunk)
	n = min(n, int(total-s.at)*s.channels)
	n -= n % s.channels
	if n <= 0 {
		return io.EOF
	}
	for i := 0; i < n; i += s.channels {
		for c := range s.win {
			s.win[c] = append(s.win[c], chunk[i+c])
		}
	}
	s.at += int64(n / s.channels)
	return nil
}

func (s *dsdStream) Read(dst []float32) (int, error) {
	if s.pos >= s.frames {
		return 0, io.EOF
	}
	n := len(s.tables)
	for len(s.win[0]) < n {
		if err := s.fill(); err != nil {
			return 0, io.EOF
		}
	}
	want := min(int64(len(dst)/2), s.frames-s.pos)
	out := dst[:0]
	for int64(len(out)/2) < want {
		if len(s.win[0]) < n {
			if err := s.fill(); err != nil {
				break
			}
			continue
		}
		var l, r float32
		for c := 0; c < min(s.channels, 2); c++ {
			w := s.win[c][:n]
			v := float32(0)
			for j, b := range w {
				v += s.tables[j][b]
			}
			if c == 0 {
				l = v
			} else {
				r = v
			}
		}
		if s.channels == 1 {
			r = l
		}
		out = append(out, l, r)
		for c := range s.win {
			s.win[c] = s.win[c][s.step:]
		}
	}
	// Keep the windows from growing past what is read next.
	for c := range s.win {
		if cap(s.win[c]) > 8*n && len(s.win[c]) < 2*n {
			s.win[c] = append(make([]byte, 0, 4*n), s.win[c]...)
		}
	}
	if len(out) == 0 {
		return 0, io.EOF
	}
	s.spec.feed(s.pos, out)
	s.pos += int64(len(out) / 2)
	return len(out), nil
}

// SetPosition goes to frame, reading on from the bytes it is made of.
func (s *dsdStream) SetPosition(frame int64) error {
	frame = max(0, min(frame, s.frames))
	s.seekBytes(frame * int64(s.step))
	s.pos = frame
	return nil
}

func (s *dsdStream) source() (*download, *spectrum) { return s.dl, s.spec }

func (s *dsdStream) Close() error {
	s.once.Do(s.drop)
	return nil
}
