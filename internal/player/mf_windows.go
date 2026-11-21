package player

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// Windows decodes what the player's own decoders cannot, AAC above all (in
// an M4A, or bare as ADTS), with Media Foundation, which every Windows 10 and
// 11 has: a viewer's cloud drive held songs saved from Bilibili as AAC,
// which stayed "loading" (1.3.2). The download reaches it through an
// IStream of Go's, and it hands back float samples.

type guid struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

var (
	iidIUnknown          = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidISequentialStream = guid{0x0c733a30, 0x2a1c, 0x11ce, [8]byte{0xad, 0xe5, 0x00, 0xaa, 0x00, 0x44, 0x77, 0x3d}}
	iidIStream           = guid{0x0000000c, 0x0000, 0x0000, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIMFAttributes     = guid{0x2cd2d921, 0xc447, 0x44a7, [8]byte{0xa1, 0x3c, 0x4a, 0xda, 0xbf, 0xc2, 0x47, 0xe3}}

	mfMediaTypeAudio  = guid{0x73647561, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfAudioFormatFlt  = guid{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfMTMajorType     = guid{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mfMTSubtype       = guid{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mfMTChannels      = guid{0x37e48bf5, 0x645e, 0x4c5b, [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	mfMTSampleRate    = guid{0x5faeeae7, 0x0290, 0x4c31, [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	mfPDDuration      = guid{0x6c990d33, 0xbb8e, 0x477a, [8]byte{0x85, 0x98, 0x0d, 0x5d, 0x96, 0xfc, 0xd8, 0x8a}}
	mfByteStreamCType = guid{0xfc358289, 0x3cb6, 0x460c, [8]byte{0xa4, 0x24, 0xb6, 0x68, 0x12, 0x60, 0x37, 0x5a}}
	guidNull          = guid{}
)

const (
	mfVersion            = 0x00020070
	mfStartupLite        = 1
	mfFirstAudioStream   = 0xfffffffd
	mfMediaSource        = 0xffffffff
	mfReaderEndOfStream  = 0x2
	mfReaderTypeChanged  = 0x20
	coinitMultithreaded  = 0
	eNoInterface         = 0x80004002
	sFalse               = 1
	vtI8                 = 20
	stgTypeStream        = 2
	streamSeekSet        = 0
	streamSeekCur        = 1
	streamSeekEnd        = 2
	hundredNsPerSecond   = 10_000_000
	sourceReaderMethods  = 13
	attributeMethodCount = 33 // IUnknown's and IMFAttributes'
)

var (
	ole32       = syscall.NewLazyDLL("ole32.dll")
	mfplat      = syscall.NewLazyDLL("mfplat.dll")
	mfreadwrite = syscall.NewLazyDLL("mfreadwrite.dll")

	procCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	procPropVariantClear   = ole32.NewProc("PropVariantClear")
	procMFStartup          = mfplat.NewProc("MFStartup")
	procMFCreateMediaType  = mfplat.NewProc("MFCreateMediaType")
	procMFByteStreamOnStrm = mfplat.NewProc("MFCreateMFByteStreamOnStream")
	procMFReaderFromStream = mfreadwrite.NewProc("MFCreateSourceReaderFromByteStream")
)

// comObject is any COM object Windows hands out: its methods' table.
type comObject struct {
	vtbl *[64]uintptr
}

func (o *comObject) call(method int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

func (o *comObject) release() {
	if o != nil {
		o.call(2)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

// mfReady starts Media Foundation once, in the process's multithreaded
// apartment, which a thread of its own joins and never leaves: the threads
// that decode belong to it then without joining.
var mfReady = sync.OnceValue(func() error {
	if err := mfplat.Load(); err != nil {
		return err
	}
	if err := mfreadwrite.Load(); err != nil {
		return err
	}
	joined := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		procCoInitializeEx.Call(0, coinitMultithreaded)
		close(joined)
		select {}
	}()
	<-joined
	if hr, _, _ := procMFStartup.Call(mfVersion, mfStartupLite); failed(hr) {
		return fmt.Errorf("mf: startup %#x", hr)
	}
	return nil
})

// goStream is an IStream over a download, for Media Foundation to read.
// COM keeps a pointer to it: streams holds it alive until its last
// Release.
type goStream struct {
	vtbl *[14]uintptr
	refs int32
	dl   *download
	pos  int64
}

var (
	streamsMu sync.Mutex
	streams   = map[*goStream]bool{}
)

type statStg struct {
	name uintptr
	typ  uint32
	_    uint32
	size uint64
	rest [56]byte // the times, the mode, the class: none
}

var goStreamVtbl = sync.OnceValue(func() *[14]uintptr {
	notImpl := syscall.NewCallback(func(this *goStream) uintptr { return 0x80004001 })
	var v [14]uintptr
	v[0] = syscall.NewCallback(func(this *goStream, riid *guid, out **goStream) uintptr {
		if *riid == iidIUnknown || *riid == iidISequentialStream || *riid == iidIStream {
			streamsMu.Lock()
			this.refs++
			streamsMu.Unlock()
			*out = this
			return 0
		}
		*out = nil
		return eNoInterface
	})
	v[1] = syscall.NewCallback(func(this *goStream) uintptr {
		streamsMu.Lock()
		defer streamsMu.Unlock()
		this.refs++
		return uintptr(this.refs)
	})
	v[2] = syscall.NewCallback(func(this *goStream) uintptr {
		streamsMu.Lock()
		defer streamsMu.Unlock()
		this.refs--
		if this.refs <= 0 {
			delete(streams, this)
			return 0
		}
		return uintptr(this.refs)
	})
	// Read: as much as is in, waiting for the download, short only at
	// its end.
	v[3] = syscall.NewCallback(func(this *goStream, pv *byte, cb uint32, read *uint32) uintptr {
		buf := unsafe.Slice(pv, cb)
		r := downloadReader{d: this.dl, pos: this.pos}
		n, _ := io.ReadFull(&r, buf)
		this.pos += int64(n)
		if read != nil {
			*read = uint32(n)
		}
		if n < int(cb) {
			return sFalse
		}
		return 0
	})
	v[4] = notImpl // Write
	v[5] = syscall.NewCallback(func(this *goStream, move int64, origin uint32, at *uint64) uintptr {
		switch origin {
		case streamSeekSet:
			this.pos = move
		case streamSeekCur:
			this.pos += move
		case streamSeekEnd:
			this.pos = this.size() + move
		}
		if this.pos < 0 {
			this.pos = 0
		}
		if at != nil {
			*at = uint64(this.pos)
		}
		return 0
	})
	for i := 6; i <= 12; i++ { // SetSize, CopyTo, Commit, Revert, LockRegion, UnlockRegion, ...
		v[i] = notImpl
	}
	v[12] = syscall.NewCallback(func(this *goStream, st *statStg, flag uint32) uintptr {
		*st = statStg{typ: stgTypeStream, size: uint64(this.size())}
		return 0
	})
	v[13] = notImpl // Clone
	return &v
})

// size is the download's whole length, waited for if it is not known.
func (s *goStream) size() int64 {
	if s.dl.size > 0 {
		return s.dl.size
	}
	return s.dl.waitFor(1 << 62)
}

// mfStream decodes a download with Media Foundation for package audio.
type mfStream struct {
	dl       *download
	reader   *comObject
	rate     int
	channels int
	frames   int64
	pos      int64 // the frame Read returns next
	skip     int64 // frames to drop, after a seek
	pend     []float32
	ended    bool
	spec     *spectrum
	drop     func()
	once     sync.Once
}

// mfContentType is the MIME type of format, as otherFormat names it, for
// Media Foundation to know how to read it.
func mfContentType(format string) string {
	switch format {
	case "AAC":
		return "audio/aac"
	case "WMA":
		return "audio/x-ms-wma"
	}
	return "audio/mp4"
}

// openMFStream opens dl, audio in format, with Media Foundation.
func openMFStream(dl *download, durationMs int64, format string) (*mfStream, error) {
	if err := mfReady(); err != nil {
		return nil, err
	}
	gs := &goStream{vtbl: goStreamVtbl(), refs: 1, dl: dl}
	streamsMu.Lock()
	streams[gs] = true
	streamsMu.Unlock()
	var bs *comObject
	hr, _, _ := procMFByteStreamOnStrm.Call(uintptr(unsafe.Pointer(gs)), uintptr(unsafe.Pointer(&bs)))
	releaseGo(gs) // the byte stream holds its own reference
	if failed(hr) || bs == nil {
		return nil, fmt.Errorf("mf: byte stream %#x", hr)
	}
	defer bs.release()
	// Its attributes say what it holds: there is no file name to go by.
	var attrs *comObject
	if !failed(bs.call(0, uintptr(unsafe.Pointer(&iidIMFAttributes)), uintptr(unsafe.Pointer(&attrs)))) && attrs != nil {
		ctype, _ := syscall.UTF16PtrFromString(mfContentType(format))
		attrs.call(25, uintptr(unsafe.Pointer(&mfByteStreamCType)), uintptr(unsafe.Pointer(ctype))) // SetString
		attrs.release()
	}
	var reader *comObject
	hr, _, _ = procMFReaderFromStream.Call(uintptr(unsafe.Pointer(bs)), 0, uintptr(unsafe.Pointer(&reader)))
	if failed(hr) || reader == nil {
		return nil, fmt.Errorf("mf: no reader for it %#x", hr)
	}
	s := &mfStream{dl: dl, reader: reader, drop: dl.Close}
	if err := s.setFloat(); err != nil {
		reader.release()
		return nil, err
	}
	if s.rate <= 0 || s.channels <= 0 {
		reader.release()
		return nil, errors.New("mf: no audio format")
	}
	var pv [24]byte
	if !failed(reader.call(12, mfMediaSource, uintptr(unsafe.Pointer(&mfPDDuration)), uintptr(unsafe.Pointer(&pv)))) {
		d := *(*uint64)(unsafe.Pointer(&pv[8]))
		s.frames = int64(d) * int64(s.rate) / hundredNsPerSecond
		procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	}
	if s.frames <= 0 && durationMs > 0 {
		s.frames = durationMs * int64(s.rate) / 1000
	}
	s.spec = dl.spectrum(s.rate, s.frames)
	return s, nil
}

func releaseGo(gs *goStream) {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	gs.refs--
	if gs.refs <= 0 {
		delete(streams, gs)
	}
}

// setFloat asks for float samples of the first audio stream, and reads
// their rate and channels.
func (s *mfStream) setFloat() error {
	var mt *comObject
	if hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&mt))); failed(hr) || mt == nil {
		return fmt.Errorf("mf: media type %#x", hr)
	}
	mt.call(24, uintptr(unsafe.Pointer(&mfMTMajorType)), uintptr(unsafe.Pointer(&mfMediaTypeAudio))) // SetGUID
	mt.call(24, uintptr(unsafe.Pointer(&mfMTSubtype)), uintptr(unsafe.Pointer(&mfAudioFormatFlt)))
	hr := s.reader.call(7, mfFirstAudioStream, 0, uintptr(unsafe.Pointer(mt))) // SetCurrentMediaType
	mt.release()
	if failed(hr) {
		return fmt.Errorf("mf: cannot decode it to float %#x", hr)
	}
	return s.readFormat()
}

func (s *mfStream) readFormat() error {
	var cur *comObject
	if hr := s.reader.call(6, mfFirstAudioStream, uintptr(unsafe.Pointer(&cur))); failed(hr) || cur == nil {
		return fmt.Errorf("mf: current type %#x", hr)
	}
	defer cur.release()
	var rate, channels uint32
	cur.call(7, uintptr(unsafe.Pointer(&mfMTSampleRate)), uintptr(unsafe.Pointer(&rate))) // GetUINT32
	cur.call(7, uintptr(unsafe.Pointer(&mfMTChannels)), uintptr(unsafe.Pointer(&channels)))
	s.rate, s.channels = int(rate), int(channels)
	return nil
}

func (s *mfStream) SampleRate() int { return s.rate }
func (s *mfStream) Channels() int   { return 2 }

func (s *mfStream) Len() int64 {
	if s.frames <= 0 {
		return -1
	}
	return s.frames
}

// next decodes the next sample into pend, in stereo, but the frames to
// skip.
func (s *mfStream) next() error {
	var index, flags uint32
	var at int64
	var sample *comObject
	hr := s.reader.call(9, mfFirstAudioStream, 0, uintptr(unsafe.Pointer(&index)), uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&at)), uintptr(unsafe.Pointer(&sample)))
	if failed(hr) {
		return fmt.Errorf("mf: read %#x", hr)
	}
	if flags&mfReaderTypeChanged != 0 {
		s.readFormat()
	}
	if sample == nil {
		if flags&mfReaderEndOfStream != 0 {
			s.ended = true
			return io.EOF
		}
		return nil
	}
	s.feedSample(sample)
	sample.release()
	return nil
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *mfStream) Read(dst []float32) (int, error) {
	for len(s.pend) == 0 {
		if s.ended {
			return 0, io.EOF
		}
		if err := s.next(); err != nil {
			return 0, err
		}
	}
	n := copy(dst[:len(dst)/2*2], s.pend)
	s.pend = s.pend[n:]
	s.spec.feed(s.pos, dst[:n])
	s.pos += int64(n / 2)
	return n, nil
}

// SetPosition goes to frame: Media Foundation goes to the sample before it,
// whose time says how much to drop.
func (s *mfStream) SetPosition(frame int64) error {
	var pv [24]byte
	*(*uint16)(unsafe.Pointer(&pv[0])) = vtI8
	*(*int64)(unsafe.Pointer(&pv[8])) = frame * hundredNsPerSecond / int64(max2(s.rate, 1))
	if hr := s.reader.call(8, uintptr(unsafe.Pointer(&guidNull)), uintptr(unsafe.Pointer(&pv))); failed(hr) {
		return fmt.Errorf("mf: seek %#x", hr)
	}
	s.pend, s.ended = s.pend[:0], false
	// The first sample after the seek: where it starts.
	var index, flags uint32
	var at int64
	var sample *comObject
	hr := s.reader.call(9, mfFirstAudioStream, 0, uintptr(unsafe.Pointer(&index)), uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&at)), uintptr(unsafe.Pointer(&sample)))
	if failed(hr) {
		return fmt.Errorf("mf: read %#x", hr)
	}
	if sample == nil {
		s.ended = flags&mfReaderEndOfStream != 0
		s.pos, s.skip = frame, 0
		return nil
	}
	start := at * int64(s.rate) / hundredNsPerSecond
	s.skip = max(0, frame-start)
	s.pos = frame
	s.feedSample(sample)
	sample.release()
	return nil
}

// feedSample puts sample's frames in pend, as next does.
func (s *mfStream) feedSample(sample *comObject) {
	var buf *comObject
	if hr := sample.call(attributeMethodCount+8, uintptr(unsafe.Pointer(&buf))); failed(hr) || buf == nil {
		return
	}
	defer buf.release()
	var data *float32
	var max, length uint32
	if hr := buf.call(3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&max)), uintptr(unsafe.Pointer(&length))); failed(hr) || data == nil {
		return
	}
	samples := unsafe.Slice(data, length/4)
	ch := max2(s.channels, 1)
	frames := len(samples) / ch
	from := min(int(s.skip), frames)
	s.skip -= int64(from)
	out := s.pend[:0]
	for i := from; i < frames; i++ {
		l := samples[i*ch]
		r := l
		if ch > 1 {
			r = samples[i*ch+1]
		}
		out = append(out, l, r)
	}
	buf.call(4)
	s.pend = out
}

func (s *mfStream) source() (*download, *spectrum) { return s.dl, s.spec }

func (s *mfStream) Close() error {
	s.once.Do(func() {
		s.reader.release()
		s.drop()
	})
	return nil
}
