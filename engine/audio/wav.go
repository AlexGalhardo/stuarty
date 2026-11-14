package audio

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// maxWAVData is the largest data chunk a RIFF header can state.
const maxWAVData = math.MaxUint32 - 64

// wavHeader is the 44 bytes before the samples of a 48 kHz stereo WAV: 16-bit PCM, or 32-bit float.
func wavHeader(frames int64, float bool) ([]byte, error) {
	bits, format := 16, 1
	if float {
		bits, format = 32, 3
	}
	frameBytes := Channels * bits / 8
	data := frames * int64(frameBytes)
	if frames < 0 || data > maxWAVData {
		return nil, fmt.Errorf("a WAV of %d frames does not fit a RIFF header", frames)
	}

	h := make([]byte, 0, 44)
	h = append(h, "RIFF"...)
	h = binary.LittleEndian.AppendUint32(h, uint32(36+data)) //nolint:gosec // bounded by maxWAVData
	h = append(h, "WAVEfmt "...)
	h = binary.LittleEndian.AppendUint32(h, 16)
	h = binary.LittleEndian.AppendUint16(h, uint16(format))
	h = binary.LittleEndian.AppendUint16(h, Channels)
	h = binary.LittleEndian.AppendUint32(h, SampleRate)
	h = binary.LittleEndian.AppendUint32(h, uint32(SampleRate*frameBytes))
	h = binary.LittleEndian.AppendUint16(h, uint16(frameBytes))
	h = binary.LittleEndian.AppendUint16(h, uint16(bits))
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, uint32(data)) //nolint:gosec // bounded by maxWAVData
	return h, nil
}

// MixToFloatWAV mixes the sources into total frames and writes them as a 32-bit float WAV, the file
// the loudness measurement reads.
func MixToFloatWAV(
	ctx context.Context,
	srcs []plan.AudioSource,
	total int64,
	open Opener,
	w io.Writer,
) error {
	h, err := wavHeader(total, true)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(w, 1<<20)
	if _, err := bw.Write(h); err != nil {
		return err
	}
	var buf []byte
	err = Mix(ctx, srcs, total, open, func(block []float32) error {
		buf = buf[:0]
		for _, v := range block {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v))
		}
		_, err := bw.Write(buf)
		return err
	})
	if err != nil {
		return err
	}
	return bw.Flush()
}

// GainToPCM16WAV reads the float WAV of MixToFloatWAV (frames frames of it), applies one gain of
// gainDB decibels and writes a 16-bit WAV, rounding to the nearest value and clamping.
func GainToPCM16WAV(r io.Reader, w io.Writer, frames int64, gainDB float64) error {
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
	in := make([]byte, 8*4096)
	var out []byte
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
		for i := 0; i < len(chunk); i += 4 {
			v := float64(math.Float32frombits(binary.LittleEndian.Uint32(chunk[i:])))
			s := math.Floor(mul(mul(v, lin), 32768) + 0.5)
			out = binary.LittleEndian.AppendUint16(out, pcm16(s))
		}
		if _, err := bw.Write(out); err != nil {
			return err
		}
		left -= n
	}
	return bw.Flush()
}

// SilentWAV writes frames frames of silence as a 16-bit WAV, the audio of a project with no source.
func SilentWAV(w io.Writer, frames int64) error {
	h, err := wavHeader(frames, false)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(w, 1<<20)
	if _, err := bw.Write(h); err != nil {
		return err
	}
	zero := make([]byte, 1<<16)
	for left := frames * 4; left > 0; {
		n := min(left, int64(len(zero)))
		if _, err := bw.Write(zero[:n]); err != nil {
			return err
		}
		left -= n
	}
	return bw.Flush()
}

// pcm16 is a sample value as the 16 bits of a 16-bit PCM sample, clamped to the range.
func pcm16(v float64) uint16 {
	return uint16(int16(min(max(v, -32768), 32767))) //nolint:gosec // clamped, then reinterpreted
}
