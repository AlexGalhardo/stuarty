package audio

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	// LimiterCeilingDBFS is the sample peak the limiter holds: one dB under PeakCeilingDBTP, because
	// the true peak of a signal can pass its sample peak between two samples.
	LimiterCeilingDBFS = PeakCeilingDBTP - 1
	// MaxLimitingDB is the most the limiter may take off a peak to reach the target loudness. Beyond
	// it the mix would be squashed, so the export stops short of the target and says so.
	MaxLimitingDB = 12
	// limiterLookahead is 5 ms: the gain starts to fall that long before a peak and rises as slowly.
	limiterLookahead = SampleRate / 200
)

// peakLimiter is a look-ahead limiter: the gain at a frame is the mean of the minimums of the gain
// each frame needs over the next limiterLookahead frames, so no frame passes the ceiling and the
// gain moves smoothly. It delays the output by limiterLookahead frames: the constructor primes it
// with that many frames of silence, and the caller feeds the same number at the end to flush it.
type peakLimiter struct {
	ceiling float64
	n       int
	in      [][2]float64 // ring of the delayed input frames
	deque   []dequeItem  // minimums of need over the window, increasing in need
	deficit []float64    // ring of 1 - minimum for the last limiterLookahead+1 windows
	sum     float64
}

type dequeItem struct {
	idx  int
	need float64
}

func newPeakLimiter() *peakLimiter {
	l := &peakLimiter{
		ceiling: math.Pow(10, LimiterCeilingDBFS/20),
		in:      make([][2]float64, limiterLookahead+1),
		deficit: make([]float64, limiterLookahead+1),
	}
	for range limiterLookahead {
		l.push(0, 0)
	}
	return l
}

// push takes one frame and returns the frame limiterLookahead frames before it, limited.
func (l *peakLimiter) push(a, b float64) (float64, float64) {
	pos := l.n
	l.in[pos%len(l.in)] = [2]float64{a, b}

	need := 1.0
	if p := math.Max(math.Abs(a), math.Abs(b)); p > l.ceiling {
		need = l.ceiling / p
	}
	for len(l.deque) > 0 && l.deque[len(l.deque)-1].need >= need {
		l.deque = l.deque[:len(l.deque)-1]
	}
	l.deque = append(l.deque, dequeItem{pos, need})
	if l.deque[0].idx <= pos-limiterLookahead-1 {
		l.deque = l.deque[1:]
	}

	slot := pos % len(l.deficit)
	l.sum -= l.deficit[slot]
	l.deficit[slot] = 1 - l.deque[0].need
	l.sum += l.deficit[slot]
	if l.sum < 1e-15 {
		l.sum = 0
	}
	l.n++

	g := 1 - l.sum/float64(len(l.deficit))
	if pos < limiterLookahead {
		return 0, 0
	}
	f := l.in[(pos-limiterLookahead)%len(l.in)]
	return f[0] * g, f[1] * g
}

// LimitToPCM16WAV is GainToPCM16WAV followed by the peak limiter: it reads the float WAV of
// MixToFloatWAV (frames frames of it), applies gainDB, holds every sample under LimiterCeilingDBFS
// and writes a 16-bit WAV.
func LimitToPCM16WAV(r io.Reader, w io.Writer, frames int64, gainDB float64) error {
	h, err := wavHeader(frames, false)
	if err != nil {
		return err
	}
	br := bufio.NewReaderSize(r, 1<<20)
	if _, err := io.CopyN(io.Discard, br, 44); err != nil {
		return fmt.Errorf("the float mix has no header: %w", err)
	}
	bw := bufio.NewWriterSize(w, 1<<20)
	if _, err := bw.Write(h); err != nil {
		return err
	}

	lin := math.Pow(10, gainDB/20)
	lim := newPeakLimiter()
	in := make([]byte, 8*4096)
	var out []byte
	emit := func(a, b float64) {
		out = binary.LittleEndian.AppendUint16(out, pcm16(math.Floor(mul(a, 32768)+0.5)))
		out = binary.LittleEndian.AppendUint16(out, pcm16(math.Floor(mul(b, 32768)+0.5)))
	}
	sample := func(b []byte) float64 {
		return mul(float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), lin)
	}

	skip := limiterLookahead
	step := func(a, b float64) {
		oa, ob := lim.push(a, b)
		if skip > 0 {
			skip--
			return
		}
		emit(oa, ob)
	}
	for left := frames; left > 0; {
		n := min(left, 4096)
		chunk := in[:n*8]
		if _, err := io.ReadFull(br, chunk); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return fmt.Errorf("the float mix is shorter than %d frames: %w", frames, err)
		}
		out = out[:0]
		for i := 0; i < len(chunk); i += 8 {
			step(sample(chunk[i:]), sample(chunk[i+4:]))
		}
		if _, err := bw.Write(out); err != nil {
			return err
		}
		left -= n
	}
	out = out[:0]
	for range limiterLookahead {
		step(0, 0)
	}
	if _, err := bw.Write(out); err != nil {
		return err
	}
	return bw.Flush()
}
