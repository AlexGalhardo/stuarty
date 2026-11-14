package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// tone is the decoded stream of a fake asset: sample i is a deterministic function of the asset and i.
func tone(asset string, n int64) []byte {
	seed := int64(0)
	for _, c := range asset {
		seed = seed*31 + int64(c)
	}
	b := make([]byte, 0, n*4)
	for i := range n {
		l := int16((seed*7 + i*13) % 20000)
		r := int16((seed*5 - i*11) % 20000)
		b = binary.LittleEndian.AppendUint16(b, uint16(l))
		b = binary.LittleEndian.AppendUint16(b, uint16(r))
	}
	return b
}

type fakeOpener struct {
	streams map[string][]byte
	opened  atomic.Int32
	closed  atomic.Int32
}

type countingCloser struct {
	io.Reader
	o *fakeOpener
}

func (c countingCloser) Close() error { c.o.closed.Add(1); return nil }

func (o *fakeOpener) Open(_ context.Context, s plan.AudioSource) (io.ReadCloser, error) {
	o.opened.Add(1)
	return countingCloser{Reader: bytes.NewReader(o.streams[s.AssetID]), o: o}, nil
}

func flat(g float64) []plan.GainPoint { return []plan.GainPoint{{Sample: 0, Gain: g}} }

func mix(t *testing.T, srcs []plan.AudioSource, total int64, o Opener) []float32 {
	t.Helper()
	var out []float32
	err := Mix(context.Background(), srcs, total, o, func(b []float32) error {
		out = append(out, b...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMixOneSourceAtUnityGain(t *testing.T) {
	t.Parallel()
	o := &fakeOpener{streams: map[string][]byte{"a": tone("a", 1000)}}
	got := mix(
		t,
		[]plan.AudioSource{{ItemID: "i", AssetID: "a", StartSample: 100, Samples: 500, Gain: flat(1)}},
		1000,
		o,
	)
	if len(got) != 2000 {
		t.Fatalf("%d values, want 2000", len(got))
	}
	raw := o.streams["a"]
	for i := range int64(1000) {
		var wl, wr float32
		if i >= 100 && i < 600 {
			j := i - 100
			wl = float32(int16(binary.LittleEndian.Uint16(raw[j*4:]))) / 32768
			wr = float32(int16(binary.LittleEndian.Uint16(raw[j*4+2:]))) / 32768
		}
		if got[i*2] != wl || got[i*2+1] != wr {
			t.Fatalf("frame %d = %v, %v want %v, %v", i, got[i*2], got[i*2+1], wl, wr)
		}
	}
	if o.opened.Load() != 1 || o.closed.Load() != 1 {
		t.Errorf("opened %d closed %d, want one each", o.opened.Load(), o.closed.Load())
	}
}

func TestMixSumsSourcesAndAppliesTheEnvelope(t *testing.T) {
	t.Parallel()
	o := &fakeOpener{streams: map[string][]byte{"a": tone("a", 100), "b": tone("b", 100)}}
	srcs := []plan.AudioSource{
		{ItemID: "1", AssetID: "a", Samples: 100, Gain: flat(0.5)},
		{ItemID: "2", AssetID: "b", Samples: 100, Gain: []plan.GainPoint{{Sample: 0, Gain: 0}, {Sample: 100, Gain: 1}}},
	}
	got := mix(t, srcs, 100, o)
	sample := func(asset string, i int) float64 {
		return float64(
			int16(binary.LittleEndian.Uint16(o.streams[asset][i*4:])),
		) / 32768
	}
	for _, i := range []int{0, 1, 50, 99} {
		want := sample("a", i)*0.5 + sample("b", i)*float64(i)/100
		if math.Abs(float64(got[i*2])-want) > 1e-6 {
			t.Errorf("frame %d = %v, want %v", i, got[i*2], want)
		}
	}
}

func TestMixPadsAShortStreamAndTruncatesALongOne(t *testing.T) {
	t.Parallel()
	o := &fakeOpener{streams: map[string][]byte{"short": tone("short", 10), "long": tone("long", 500)}}
	srcs := []plan.AudioSource{
		{ItemID: "1", AssetID: "short", Samples: 100, Gain: flat(1)},
		{ItemID: "2", AssetID: "long", StartSample: 100, Samples: 50, Gain: flat(1)},
	}
	got := mix(t, srcs, 200, o)
	for i := 10; i < 100; i++ {
		if got[i*2] != 0 || got[i*2+1] != 0 {
			t.Fatalf("frame %d should be padded with silence, got %v", i, got[i*2])
		}
	}
	for i := 150; i < 200; i++ {
		if got[i*2] != 0 {
			t.Fatalf("frame %d is past the end of the source", i)
		}
	}
	if got[100*2] == 0 && got[100*2+1] == 0 {
		t.Error("the second source never sounded")
	}
	if o.opened.Load() != o.closed.Load() {
		t.Errorf("opened %d, closed %d", o.opened.Load(), o.closed.Load())
	}
}

// A source that spans several blocks gives the same samples as the same source in one block.
func TestMixAcrossBlocks(t *testing.T) {
	t.Parallel()
	n := int64(BlockSamples*2 + 777)
	o := &fakeOpener{streams: map[string][]byte{"a": tone("a", n)}}
	src := plan.AudioSource{ItemID: "1", AssetID: "a", StartSample: 5, Samples: n - 5, Gain: []plan.GainPoint{
		{Sample: 0, Gain: 0.25}, {Sample: BlockSamples + 3, Gain: 1}, {Sample: n, Gain: 0.5},
	}}
	got := mix(t, []plan.AudioSource{src}, n, o)
	raw := o.streams["a"]
	for _, i := range []int64{5, 6, BlockSamples - 1, BlockSamples, BlockSamples + 8, n - 1} {
		p := i - 5
		var g float64
		switch {
		case p <= BlockSamples+3:
			g = 0.25 + (1-0.25)*float64(p)/float64(BlockSamples+3)
		default:
			g = 1 + (0.5-1)*float64(p-(BlockSamples+3))/float64(n-(BlockSamples+3))
		}
		want := float64(int16(binary.LittleEndian.Uint16(raw[p*4:]))) / 32768 * g
		if math.Abs(float64(got[i*2])-want) > 1e-6 {
			t.Errorf("frame %d = %v, want %v", i, got[i*2], want)
		}
	}
}

func TestMixStopsOnCancelAndClosesStreams(t *testing.T) {
	t.Parallel()
	n := int64(BlockSamples * 3)
	o := &fakeOpener{streams: map[string][]byte{"a": tone("a", n)}}
	ctx, cancel := context.WithCancel(context.Background())
	blocks := 0
	err := Mix(
		ctx,
		[]plan.AudioSource{{ItemID: "1", AssetID: "a", Samples: n, Gain: flat(1)}},
		n,
		o,
		func([]float32) error {
			blocks++
			cancel()
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) || blocks != 1 {
		t.Fatalf("err = %v after %d blocks", err, blocks)
	}
	if o.closed.Load() != 1 {
		t.Errorf("the stream was not closed")
	}
}

// The mix of these cases is pinned by hash: any change in the arithmetic shows, on every architecture.
func TestMixEqualsItsGoldenHash(t *testing.T) {
	t.Parallel()
	n := int64(BlockSamples + 12345)
	o := &fakeOpener{streams: map[string][]byte{"a": tone("a", n), "b": tone("b", n), "c": tone("c", n)}}
	srcs := []plan.AudioSource{
		{ItemID: "1", AssetID: "a", Samples: n, Gain: flat(0.7)},
		{ItemID: "2", AssetID: "b", StartSample: 1000, Samples: n - 2000, Gain: []plan.GainPoint{
			{Sample: 0, Gain: 0}, {Sample: 4800, Gain: 1}, {Sample: n - 6800, Gain: 1}, {Sample: n - 2000, Gain: 0},
		}},
		{ItemID: "3", AssetID: "c", StartSample: 30000, Samples: 20000, Gain: flat(1.3)},
	}
	var buf bytes.Buffer
	if err := MixToFloatWAV(context.Background(), srcs, n, o, &buf); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	const want = "d61df960724620ba68800edae9c73155c322a21dd09be2580faac6a19a55574f"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("hash = %s", got)
	}
}
