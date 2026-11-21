package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// snapshot reads the names that "ffmpeg -demuxers" or "-filters" of the pinned build listed (SDD-007 FF-04,
// FF-10): the lists the allowlists were derived from, kept so that a pin bump shows what it added.
func snapshot(t *testing.T, name string) map[string]bool {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", "builds", "btbn", name))
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}

	for n := range strings.SplitSeq(string(b), "\n") {
		if n = strings.TrimSpace(n); n != "" {
			names[n] = true
		}
	}

	return names
}

func listing(t *testing.T, flag string) map[string]bool {
	t.Helper()

	r, dir := pinnedRunner(t)

	argv, err := ffargs.Info(flag)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if _, err := r.RunFFmpeg(context.Background(), Request{
		Dir: dir, Argv: argv, Deadline: 30 * time.Second, Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}

	return parseListing(out.String())
}

// Every demuxer of the input allowlist and every filter of the grammar is in the snapshot of the pinned
// build, with no FFmpeg needed.
func TestAllowlistsAreInsideTheBuildSnapshots(t *testing.T) {
	t.Parallel()

	demuxers, filters := snapshot(t, "demuxers.txt"), snapshot(t, "filters.txt")

	have := map[string]bool{}
	for d := range demuxers {
		for part := range strings.SplitSeq(d, ",") {
			have[part] = true
		}
	}

	for n := range strings.SplitSeq(ffargs.InputFormats, ",") {
		if !have[n] {
			t.Errorf("the allowed input format %s is not in the demuxer snapshot", n)
		}
	}

	for _, n := range ffargs.FilterNames() {
		if !filters[n] && !optionalFilters[n] {
			t.Errorf("the filter %s of the grammar is not in the filter snapshot", n)
		}
	}

	// What the BETA features added to the grammar.
	for _, n := range []string{"afftdn", "silencedetect", "ebur128", "aresample", "adelay", "apad"} {
		if !filters[n] {
			t.Errorf("%s is not in the filter snapshot", n)
		}
	}

	for _, n := range []string{"matroska,webm", "apng", "webp_anim", "gif", "image2"} {
		if !demuxers[n] {
			t.Errorf("%s is not in the demuxer snapshot", n)
		}
	}
}

// windowsOnlyFilters are the filters the snapshot (read on the Windows build) has because that build links
// Direct3D 11/12, AMD AMF and the Windows desktop capture APIs; the Linux build of the same pin lacks them.
// Any other difference still fails on every system.
var windowsOnlyFilters = map[string]bool{
	"ddagrab": true, "gfxcapture": true, "scale_d3d11": true, "scale_d3d12": true,
	"deinterlace_d3d12": true, "mestimate_d3d12": true, "frc_amf": true,
}

// The snapshots are the listings of the pinned build, name for name. A pin bump that adds or drops a demuxer
// or a filter fails here until the new names are read against FF-04 and FF-10 and the snapshot is rewritten
// from "ffmpeg -demuxers" and "ffmpeg -filters".
func TestSnapshotsAreTheListingsOfThePinnedBuild(t *testing.T) {
	for _, c := range []struct{ flag, file string }{{"-demuxers", "demuxers.txt"}, {"-filters", "filters.txt"}} {
		want, got := snapshot(t, c.file), listing(t, c.flag)

		for n := range got {
			if !want[n] {
				t.Errorf("%s: %s is new in the pinned build and not in the snapshot", c.flag, n)
			}
		}

		for n := range want {
			if !got[n] && !(c.flag == "-filters" && runtime.GOOS != "windows" && windowsOnlyFilters[n]) {
				t.Errorf("%s: %s is in the snapshot and gone from the pinned build", c.flag, n)
			}
		}
	}
}
