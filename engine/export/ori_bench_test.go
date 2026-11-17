package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// orientationDoc is a project of w by h at 30 fps with one clip of cw by ch (fit cover) over secs
// seconds: a landscape source in a landscape and in a portrait project is the same content with the
// same pixel count, so the two exports should cost the same.
func orientationDoc(tb testing.TB, w, h, cw, ch, secs int) *timeline.Document {
	tb.Helper()

	frames := secs * 30
	data := fmt.Sprintf(`{
  "schema_version": 1, "id": "prj_p3r4f5g6h7", "name": "Orientation",
  "settings": {"width": %d, "height": %d, "fps": {"num": 30, "den": 1}},
  "assets": {"ast_p1p1p1p1p1": {"id": "ast_p1p1p1p1p1", "hash": "sha256:%s", "kind": "video", "name": "o.mp4",
    "frame_rate": {"num": 30, "den": 1}, "frame_count": %d, "width": %d, "height": %d}},
  "tracks": {"trk_v1v1v1v1v1": {"id": "trk_v1v1v1v1v1", "kind": "visual", "name": "Video", "order": "a1"}},
  "items": {"itm_m1m1m1m1m1": {"id": "itm_m1m1m1m1m1", "type": "clip", "track": "trk_v1v1v1v1v1", "start": 0,
    "duration": %d, "asset": "ast_p1p1p1p1p1"}}
}`, w, h, strings.Repeat("0", 64), frames, cw, ch, frames)

	doc, err := timeline.Decode([]byte(data))
	if err != nil {
		tb.Fatal(err)
	}

	spec := fixtureSpec{
		Name: fmt.Sprintf("orient-%dx%d.mp4", cw, ch), Width: cw, Height: ch, FPS: 30, Seconds: float64(secs),
	}
	useAsset(doc, "ast_p1p1p1p1p1", ingestClip(tb, spec))

	return doc
}

// BenchmarkExportOrientation exports the same content at 1920x1080 and at 1080x1920 (VV-162) and
// reports the seconds each took; the two must be within a small factor of each other.
func BenchmarkExportOrientation(b *testing.B) {
	if raceEnabled {
		b.Skip("the race detector multiplies memory and time")
	}

	for _, sz := range [][2]int{{1920, 1080}, {1080, 1920}, {1080, 1080}} {
		b.Run(fmt.Sprintf("%dx%d", sz[0], sz[1]), func(b *testing.B) {
			doc := orientationDoc(b, sz[0], sz[1], 1920, 1080, 10)

			b.ResetTimer()

			for i := range b.N {
				job := newJob(b, doc, "ori-"+strconv.Itoa(i), filepath.Join("ori", "out.mp4"))
				start := time.Now()

				res, err := Run(context.Background(), job)
				if err != nil {
					b.Fatal(err)
				}

				b.ReportMetric(time.Since(start).Seconds(), "s_export")
				_ = os.Remove(res.Path)
			}
		})
	}
}
