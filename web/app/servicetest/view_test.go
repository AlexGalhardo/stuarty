package servicetest

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

func TestViewFrameAndSheetRenderTheCurrentRevision(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable() // one text item of 1 s at 30 fps, revision 2
	ctx := m.as(ui)

	f, err := m.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: project, At: "0.5s"})
	if err != nil {
		t.Fatal(err)
	}

	if f.Mode != "frame" || f.Revision != 2 || f.MIME != "image/jpeg" || !bytes.HasPrefix(f.Image, []byte{0xff, 0xd8}) ||
		f.ImageTokens > 800 || len(f.Cells) != 1 ||
		f.Cells[0].At.Frame != 15 {
		t.Fatalf("frame = %+v", f)
	}

	if _, err := os.Stat(f.Path); err != nil || !strings.HasPrefix(f.Text, "frame 0.5s/15f of "+project+" r2: ") ||
		!strings.Contains(f.Text, ", saved "+f.Path) {
		t.Errorf("path %q, %v; text %q", f.Path, err, f.Text)
	}

	again, err := m.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: project, At: "0.5s"})
	if err != nil || !bytes.Equal(again.Image, f.Image) {
		t.Errorf("the cached frame differs: %v", err)
	}

	for name, req := range map[string]service.ViewFrameRequest{
		"past the end":       {ProjectID: project, At: "5s"},
		"not a time":         {ProjectID: project, At: "soon"},
		"no time":            {ProjectID: project},
		"width and tokens":   {ProjectID: project, At: "0s", Width: 200, MaxTokens: 300},
		"odd width":          {ProjectID: project, At: "0s", Width: 201},
		"too few tokens":     {ProjectID: project, At: "0s", MaxTokens: 10},
		"an asset":           {ProjectID: project, At: "0s", AssetID: "ast_none"},
		"an unknown project": {ProjectID: "prj_unknown001", At: "0s"},
	} {
		if _, err := m.svc.ViewFrame(ctx, req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if se := serviceError(t, func() error {
		_, err := m.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: project, At: "5s"})
		return err
	}()); se.Code != "E_INVALID_TIME" || se.Hint == "" {
		t.Errorf("past the end: %+v", se)
	}

	w, err := m.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: project, At: "0s", Width: 200})
	if err != nil || w.Width != 200 {
		t.Errorf("width 200: %+v, %v", w, err)
	}

	s, err := m.svc.ViewSheet(ctx, service.ViewSheetRequest{ProjectID: project, Count: 4})
	if err != nil {
		t.Fatal(err)
	}

	if s.Mode != "sheet" || len(s.Cells) != 4 || s.ImageTokens > 1500 || !bytes.HasPrefix(s.Image, []byte{0xff, 0xd8}) ||
		s.Cells[3].At.Frame != 29 ||
		!strings.HasPrefix(s.Text, "sheet 2x2 of "+project+" r2: ") ||
		!strings.Contains(s.Text, "\ncells: 1=0s/0f") {
		t.Errorf("sheet = %+v\n%s", s, s.Text)
	}

	if _, err := os.Stat(s.Path); err != nil {
		t.Errorf("sheet path %q: %v", s.Path, err)
	}

	if _, err := m.svc.ViewSheet(ctx, service.ViewSheetRequest{ProjectID: project, Times: []string{"0s"}}); err == nil {
		t.Error("a sheet of one time was accepted")
	}

	if _, err := m.svc.ViewSheet(
		ctx,
		service.ViewSheetRequest{ProjectID: project, Times: []string{"0s", "1s"}, Count: 3},
	); err == nil {
		t.Error("times with count was accepted")
	}
}
