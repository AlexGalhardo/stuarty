package ingest

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTile(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 80, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestBuildSheetLayout(t *testing.T) {
	plan := PlanThumbnails(45_000, 360, 640) // 45 tiles at 1 s: two rows of 20 and one of 5
	dir := t.TempDir()
	var tiles []string
	for i := range 45 {
		p := filepath.Join(dir, "t"+itoa(i)+".jpg")
		writeTile(t, p, plan.TileW, plan.TileH)
		tiles = append(tiles, p)
	}
	data, l, err := BuildSheet(tiles, plan)
	if err != nil {
		t.Fatal(err)
	}
	if l.Columns != 20 || l.Count != 45 || l.Rows() != 3 || l.IntervalMS != 1000 {
		t.Errorf("layout %+v", l)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != 20*plan.TileW || cfg.Height != 3*plan.TileH {
		t.Errorf("sheet %dx%d, %v", cfg.Width, cfg.Height, err)
	}
}

// TestBuildSheetRefusesMoreTilesThanPlanned is the regression of the security review: the sheet is
// sized from the tile count, so a crafted file that makes FFmpeg write many tiles must be refused
// before anything is allocated.
func TestBuildSheetRefusesMoreTilesThanPlanned(t *testing.T) {
	plan := PlanThumbnails(10_000, 360, 640) // 10 tiles planned
	if plan.Planned != 11 {
		t.Fatalf("planned = %d", plan.Planned)
	}
	// Paths that do not exist: a refusal that decoded or even opened them would fail differently.
	fake := make([]string, 5_000_000)
	for i := range fake {
		fake[i] = "missing"
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, _, err := BuildSheet(fake, plan)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooManyTiles) {
		t.Fatalf("error = %v, want ErrTooManyTiles", err)
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 1<<20 {
		t.Errorf("the refusal allocated %d bytes", grown)
	}
	// One over the plan plus its margin is refused; the plan itself is not.
	if _, _, err := BuildSheet(fake[:plan.Planned+2], plan); !errors.Is(err, ErrTooManyTiles) {
		t.Errorf("planned+2: %v", err)
	}
	if _, _, err := BuildSheet(
		fake[:ThumbMaxCount+1],
		PlanThumbnails(10_000_000, 360, 640),
	); !errors.Is(
		err,
		ErrTooManyTiles,
	) {
		t.Errorf("over the hard cap: %v", err)
	}
	// A tile that declares a huge size is refused from its header, without decoding it.
	huge := filepath.Join(t.TempDir(), "huge.jpg")
	writeTile(t, huge, 4000, 4000)
	if _, _, err := BuildSheet([]string{huge}, PlanThumbnails(1000, 360, 640)); err == nil {
		t.Error("an oversized tile was accepted")
	}
}

func TestPlannedTilesBound(t *testing.T) {
	for _, c := range []struct {
		ms      int64
		planned int
	}{{1000, 2}, {60_000, 61}, {400_000, 401 - 1}, {10_000_000, ThumbMaxCount}} {
		if got := PlanThumbnails(c.ms, 360, 640).Planned; got != min(c.planned, ThumbMaxCount) {
			t.Errorf("%d ms: planned %d, want %d", c.ms, got, min(c.planned, ThumbMaxCount))
		}
	}
}
