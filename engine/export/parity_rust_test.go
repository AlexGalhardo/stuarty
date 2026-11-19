package export

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/bitmap"
	"github.com/AlexGalhardo/stuarty/engine/compositor"
	"github.com/AlexGalhardo/stuarty/engine/lut"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// The parity gate of stage C (SDD-013 section 3.10, skill renderer-parity): every golden case of this
// package is composed by the Go compositor and by the Rust CPU path from the same plan and the same
// source pixels, and the two frames must be the same bytes, drawn whole and drawn in eight bands.
// The Rust side is the native process vr-parity of renderer/ (cargo builds it on first use); a machine
// without cargo skips the test, a CI job with the toolchain runs it.

const (
	envParityBin  = "STUARTY_TEST_VR_PARITY"
	parityBands   = 8
	rendererRoot  = "../../renderer"
	corpusMagic   = "VVPC"
	corpusVersion = 2
)

// parityTexture and parityBitmap are what the Go compositor was given for one frame.
type parityTexture struct {
	id     string
	w, h   int
	opaque bool
	pix    []byte
}

type parityFrame struct {
	name     string
	frame    int
	w, h     int
	plan     plan.FramePlan
	scale    float64
	input    []byte
	bands    [][]byte
	textures []parityTexture
	bitmaps  map[string]bitmap.Bitmap
	luts     map[string]*lut.Table
	want     []byte
}

// parityBinary returns the path of vr-parity: the one the environment names or a fresh release build.
func parityBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv(envParityBin); p != "" {
		return p
	}
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("no cargo on the PATH, and " + envParityBin + " is not set")
	}
	root, err := filepath.Abs(rendererRoot)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(
		t.Context(),
		cargo,
		"build",
		"--locked",
		"--release",
		"--bin",
		"vr-parity",
		"--manifest-path",
		filepath.Join(root, "Cargo.toml"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cargo build of vr-parity: %v\n%s", err, out)
	}
	name := "vr-parity"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, "target", "release", name)
}

// renderInput is the render input of SDD-013 section 2.1 for a frame plan, for the rows of one band when
// y1 > y0.
func renderInput(t *testing.T, fp plan.FramePlan, w, h int, scale float64, y0, y1 int) []byte {
	t.Helper()
	in, err := json.Marshal(
		renderhost.RenderInput{Output: renderhost.Output{Width: w, Height: h, Scale: scale, Y0: y0, Y1: y1}, Plan: fp},
	)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// recordFrames composes the frames of a case with the Go compositor and records, for each, the plan and
// the pixels the compositor was given.
func recordFrames(t *testing.T, dir, name string, frames []int) []parityFrame {
	t.Helper()
	doc := loadDir(t, dir)
	job := newJob(t, doc, "parity-"+name, "unused.mp4")
	s := testState(t, job, job.JobID)
	rd := s.newRenderer(0)
	defer rd.close()

	var (
		textures []parityTexture
		bitmaps  map[string]bitmap.Bitmap
		luts     map[string]*lut.Table
	)
	media, bm, lutOf := rd.comp.Media, rd.comp.Bitmaps, rd.comp.LUTs
	rd.comp.Media = func(l *plan.Layer) (compositor.Image, error) {
		img, err := media(l)
		if err == nil {
			textures = append(
				textures,
				parityTexture{l.ItemID, img.W, img.H, img.Opaque, bytes.Clone(img.Pix[:img.W*img.H*4])},
			)
		}
		return img, err
	}
	rd.comp.Bitmaps = func(spec plan.BitmapSpec) (bitmap.Bitmap, error) {
		b, err := bm(spec)
		if err == nil {
			bitmaps[spec.Key()] = b
		}
		return b, err
	}

	rd.comp.LUTs = func(asset string) (*lut.Table, error) {
		t, err := lutOf(asset)
		if err == nil {
			luts[asset] = t
		}
		return t, err
	}

	w, h := doc.Settings.Width, doc.Settings.Height
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	var out []parityFrame
	for _, f := range frames {
		textures, bitmaps, luts = nil, map[string]bitmap.Bitmap{}, map[string]*lut.Table{}
		if err := rd.render(dst, f); err != nil {
			t.Fatalf("%s frame %d: %v", name, f, err)
		}
		fp := plan.FrameWith(doc, f, s.opts)
		pf := parityFrame{
			name:     name,
			frame:    f,
			w:        w,
			h:        h,
			textures: textures,
			bitmaps:  bitmaps,
			luts:     luts,
			want:     bytes.Clone(dst.Pix),
			plan:     fp,
			scale:    s.scale,
			input:    renderInput(t, fp, w, h, s.scale, 0, 0),
		}
		for i := range parityBands {
			pf.bands = append(pf.bands, renderInput(t, fp, w, h, s.scale, h*i/parityBands, h*(i+1)/parityBands))
		}
		out = append(out, pf)
	}
	return out
}

func le(buf *bytes.Buffer, v any) { _ = binary.Write(buf, binary.LittleEndian, v) }

// writeCorpus writes the frames in the format that vr-parity reads, each once whole and once in bands.
func writeCorpus(t *testing.T, path string, frames []parityFrame, banded bool) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	var head bytes.Buffer
	head.WriteString(corpusMagic)
	le(&head, uint32(corpusVersion))
	le(&head, uint32(len(frames)))
	_, _ = w.Write(head.Bytes())

	for i := range frames {
		fr := &frames[i]
		var b bytes.Buffer
		inputs := [][]byte{fr.input}
		if banded {
			inputs = fr.bands
		}
		le(&b, uint32(len(inputs)))
		for _, in := range inputs {
			le(&b, uint32(len(in)))
			b.Write(in)
		}
		le(&b, uint32(len(fr.textures)))
		for _, tx := range fr.textures {
			b.WriteByte(byte(len(tx.id)))
			b.WriteString(tx.id)
			le(&b, uint32(tx.w))
			le(&b, uint32(tx.h))
			if tx.opaque {
				b.WriteByte(1)
			} else {
				b.WriteByte(0)
			}
			b.Write(tx.pix)
		}
		le(&b, uint32(len(fr.bitmaps)))
		for key, bm := range fr.bitmaps {
			b.WriteByte(byte(len(key)))
			b.WriteString(key)
			le(&b, uint32(bm.Width))
			le(&b, uint32(bm.Height))
			for _, v := range []float64{bm.NaturalW, bm.NaturalH, bm.CenterDX, bm.CenterDY} {
				le(&b, v)
			}
			b.Write(bm.Pix[:bm.Width*bm.Height*4])
		}
		le(&b, uint32(len(fr.luts)))
		for asset, tb := range fr.luts {
			b.WriteByte(byte(len(asset)))
			b.WriteString(asset)
			le(&b, uint32(tb.Size))
			b.Write(tb.Bytes())
		}
		if _, err := w.Write(b.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
}

// runRust renders the corpus with vr-parity and returns the frames it wrote.
func runRust(t *testing.T, bin, corpus string, frames []parityFrame) [][]byte {
	t.Helper()
	out := filepath.Join(filepath.Dir(corpus), "frames.bin")
	cmd := exec.CommandContext(t.Context(), bin, corpus, out)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("vr-parity: %v: %s", err, stderr.String())
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	var got [][]byte
	for i := range frames {
		fr := &frames[i]
		pix := make([]byte, fr.w*fr.h*4)
		if _, err := io.ReadFull(r, pix); err != nil {
			t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
		}
		got = append(got, pix)
	}
	return got
}

func firstDiff(a, b []byte, w int) (pos string, n int) {
	for i := range a {
		if a[i] != b[i] {
			if n == 0 {
				pos = "pixel (" + strconv.Itoa(
					i/4%w,
				) + "," + strconv.Itoa(
					i/4/w,
				) + ") channel " + strconv.Itoa(
					i%4,
				) + ": " + strconv.Itoa(
					int(a[i]),
				) + " against " + strconv.Itoa(
					int(b[i]),
				)
			}
			n++
		}
	}
	return pos, n
}

// wasmFrame renders a frame with the core module under wazero: one instance for the whole frame, or one per
// band as the export does (SDD-013 section 4.6), each holding the textures and bitmaps of the frame.
func wasmFrame(t *testing.T, fr *parityFrame, banded bool) []byte {
	t.Helper()
	ctx := context.Background()
	dst := image.NewRGBA(image.Rect(0, 0, fr.w, fr.h))
	n := 1
	if banded {
		n = parityBands
	}
	for i := range n {
		h, err := renderhost.New(ctx, renderhost.Config{})
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range fr.textures {
			if err := h.PutTexture(ctx, tx.id, tx.w, tx.h, tx.pix, tx.opaque); err != nil {
				t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
			}
		}
		for key, bm := range fr.bitmaps {
			if err := h.PutBitmap(ctx, key, bm); err != nil {
				t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
			}
		}
		for asset, tb := range fr.luts {
			if err := h.PutLUT(ctx, asset, tb); err != nil {
				t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
			}
		}
		out := renderhost.Output{Width: fr.w, Height: fr.h, Scale: fr.scale}
		if banded {
			out.Y0, out.Y1 = fr.h*i/parityBands, fr.h*(i+1)/parityBands
		}
		if err := h.Render(ctx, renderhost.RenderInput{Output: out, Plan: fr.plan}, dst); err != nil {
			t.Fatalf("%s frame %d: %v", fr.name, fr.frame, err)
		}
		if err := h.Close(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return dst.Pix
}

// TestRustCPUPathEqualsGoCompositor is the gate of skill renderer-parity, part 1: not one byte differs
// between the Go compositor and the Rust CPU path, as a native process and as the WebAssembly module under
// wazero, on any frame of any golden case, drawn whole and in eight bands.
func TestRustCPUPathEqualsGoCompositor(t *testing.T) {
	setup(t)

	var all []parityFrame
	for _, name := range requiredCases {
		all = append(all, recordFrames(t, goldenDir(name), name, readFrames(t, goldenDir(name)))...)
	}
	// The reference scenes: a caption over a clip, a cut between two clips, text on a contained clip.
	for _, name := range referenceScenes {
		dir := filepath.Join("testdata", "scenes", name)
		var frames []int
		for i, f := range sceneFrames(endFrame(loadDir(t, dir))) {
			if i%5 == 0 {
				frames = append(frames, f)
			}
		}
		all = append(all, recordFrames(t, dir, "scene-"+name, frames)...)
	}

	report := func(t *testing.T, label string, got [][]byte) {
		t.Helper()
		bad := 0
		for i := range all {
			fr := &all[i]
			if pos, n := firstDiff(got[i], fr.want, fr.w); n != 0 {
				bad++
				t.Errorf("%s, %s, frame %d: %d bytes differ, first at %s", label, fr.name, fr.frame, n, pos)
			}
		}
		t.Logf(
			"%s: %d frames of %d cases and %d scenes, %d differ",
			label,
			len(all),
			len(requiredCases),
			len(referenceScenes),
			bad,
		)
	}

	t.Run("native", func(t *testing.T) {
		bin := parityBinary(t)
		for _, banded := range []bool{false, true} {
			label := "native, whole frames"
			if banded {
				label = "native, eight bands"
			}
			corpus := filepath.Join(t.TempDir(), "corpus.bin")
			writeCorpus(t, corpus, all, banded)
			report(t, label, runRust(t, bin, corpus, all))
		}
	})

	t.Run("wasm", func(t *testing.T) {
		for _, banded := range []bool{false, true} {
			label := "wasm, whole frames"
			if banded {
				label = "wasm, eight bands (one instance each)"
			}
			got := make([][]byte, len(all))
			for i := range all {
				got[i] = wasmFrame(t, &all[i], banded)
			}
			report(t, label, got)
		}
	})
}
