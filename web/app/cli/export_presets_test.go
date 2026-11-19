package cli

import (
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// TestExportBatchTakesEveryPreset: export:batch --preset a --preset b is one export per preset, in order, each with
// the shared codec and range. A flag that kept only its last value turned a batch of three into a batch of one.
func TestExportBatchTakesEveryPreset(t *testing.T) {
	t.Parallel()

	p, err := parseArgs(exportBatch.flags, []string{
		"--preset",
		"tiktok",
		"--preset=facebook-reels",
		"--preset",
		"youtube-shorts",
		"--codec",
		"hevc",
		"--end",
		"10s",
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err := batchItems(p)
	if err != nil {
		t.Fatal(err)
	}

	want := []service.ExportBatchItem{
		{Preset: "tiktok", Codec: "hevc", End: "10s"},
		{Preset: "facebook-reels", Codec: "hevc", End: "10s"},
		{Preset: "youtube-shorts", Codec: "hevc", End: "10s"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items %+v, want %+v", items, want)
	}
}
