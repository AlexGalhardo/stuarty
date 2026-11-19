package servicetest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

// notOpaque makes the PNG encoder write an alpha channel for a picture whose every pixel is opaque: the encoder
// drops the channel of an image that says it is opaque, and the point of the fixture is a file that declares one.
type notOpaque struct{ *image.NRGBA }

func (notOpaque) Opaque() bool { return false }

// alphaPNG is a 32x16 PNG that declares an alpha channel. With hole, one pixel is transparent.
func alphaPNG(t *testing.T, hole bool) string {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = 255, 255
	}

	if hole {
		img.SetNRGBA(3, 3, color.NRGBA{})
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, notOpaque{img}); err != nil {
		t.Fatal(err)
	}

	return buf.String()
}

// SDD-011 section 4.3.6, SDD-003 section 3.6: a file that declares alpha and whose pictures are all opaque makes the
// clips that use it raise L_ALPHA_MISSING through Service.Lint, from the fact the ingest stored; a file with a
// transparent pixel does not.
func TestAlphaMissingIsReportedFromTheStoredFact(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})

	for name, c := range map[string]struct {
		hole bool
		want bool
	}{"declared alpha, all opaque": {false, true}, "has a transparent pixel": {true, false}} {
		project, track := m.withTrack()
		m.importOne(project, m.write("alpha.png", alphaPNG(t, c.hole)))

		doc, err := timeline.Decode(m.document(project))
		if err != nil {
			t.Fatal(err)
		}

		var asset timeline.Asset
		for _, a := range doc.Assets {
			asset = a
		}

		if !asset.HasAlpha {
			t.Fatalf("%s: the ingest did not see the alpha channel: %+v", name, asset)
		}

		m.mustEdit(ui, project, m.head(project).Revision, op("clip.add", fmt.Sprintf(
			`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, track, asset.ID,
		)))

		out, lerr := m.svc.Lint(m.as(ui), service.LintRequest{ProjectID: project, Limit: 100})
		if lerr != nil {
			t.Fatal(lerr)
		}

		if got := strings.Contains(out.Text, "L_ALPHA_MISSING"); got != c.want {
			t.Errorf("%s: L_ALPHA_MISSING %v, want %v:\n%s", name, got, c.want, out.Text)
		}
	}
}
