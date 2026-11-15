package export

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// maxReadReader records the largest buffer one Read was handed.
type maxReadReader struct {
	r   io.Reader
	max int
}

func (m *maxReadReader) Read(p []byte) (int, error) {
	m.max = max(m.max, len(p))

	return m.r.Read(p)
}

func TestReadFrameReadsInChunks(t *testing.T) {
	t.Parallel()

	want := make([]byte, 3*pipeReadChunk+123)
	for i := range want {
		want[i] = byte(i * 7)
	}

	src := &maxReadReader{r: bytes.NewReader(want)}
	got := make([]byte, len(want))

	if err := readFrame(src, got); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatal("the frame read differs from the bytes written")
	}

	if src.max > pipeReadChunk {
		t.Fatalf("a read was handed %d bytes, more than the %d chunk", src.max, pipeReadChunk)
	}
}

func TestReadFrameShortInputFails(t *testing.T) {
	t.Parallel()

	err := readFrame(bytes.NewReader(make([]byte, pipeReadChunk+5)), make([]byte, 2*pipeReadChunk))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want io.ErrUnexpectedEOF", err)
	}

	if err := readFrame(bytes.NewReader(nil), make([]byte, 8)); !errors.Is(err, io.EOF) {
		t.Fatalf("got %v, want io.EOF", err)
	}
}
