package gioapp

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"time"
)

// speaker plays 48 kHz stereo s16le by handing it to ffmpeg's audio device.
// A machine with no device leaves playback picture-only.
type speaker struct {
	in   io.WriteCloser
	done chan struct{}
}

func openSpeaker(ctx context.Context, bin string) (*speaker, error) {
	devices := linuxAudioDevices()
	if len(devices) == 0 || bin == "" {
		return nil, errors.New("no audio device")
	}

	var last error

	for _, dev := range devices {
		sp, err := startSpeaker(ctx, bin, dev[0], dev[1])
		if err != nil {
			last = err

			continue
		}

		return sp, nil
	}

	if last == nil {
		last = errors.New("no audio device")
	}

	return nil, last
}

func startSpeaker(ctx context.Context, bin, format, device string) (*speaker, error) {
	// The arguments are fixed: raw stereo into one of the devices listed above.
	cmd := exec.CommandContext(
		ctx, bin,
		"-hide_banner", "-nostats", "-loglevel", "error",
		"-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0",
		"-f", format, device,
	)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = in.Close()

		return nil, err
	}

	done := make(chan struct{})
	exited := make(chan struct{})

	go func() {
		_ = cmd.Wait()
		close(exited)
		close(done)
	}()

	select {
	case <-exited:
		_ = in.Close()

		return nil, errors.New(format + " exited at once")
	case <-time.After(60 * time.Millisecond):
		return &speaker{in: in, done: done}, nil
	}
}

func (s *speaker) write(block []float32) error {
	if s == nil || s.in == nil {
		return errors.New("no speaker")
	}

	buf := make([]byte, len(block)*2)

	for i, sample := range block {
		v := min(max(int(sample*32767), -32768), 32767)

		binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(v))) //nolint:gosec // clamped to int16
	}

	_, err := s.in.Write(buf)

	return err
}

func (s *speaker) close() {
	if s == nil {
		return
	}

	if s.in != nil {
		_ = s.in.Close()
	}

	if s.done != nil {
		<-s.done
	}
}
