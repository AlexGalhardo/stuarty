package servicetest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// A sheet of an empty timeline is refused, and the refusal for times with a
// range names the real argument, until.
func TestViewSheetRefusesAnEmptyTimelineAndNamesUntil(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	ctx := m.as(ui)

	_, err := m.svc.ViewSheet(ctx, service.ViewSheetRequest{ProjectID: m.newProject("empty")})
	if se := serviceError(t, err); se.Code != command.CodeValidation || !strings.Contains(se.Message, "empty") {
		t.Errorf("an empty timeline = %+v", se)
	}

	_, err = m.svc.ViewSheet(ctx, service.ViewSheetRequest{
		ProjectID: m.exportable(), Times: []string{"0s", "0.5s"}, Until: "1s",
	})
	if se := serviceError(t, err); !strings.Contains(se.Message, "until") || strings.Contains(se.Message, " to ") {
		t.Errorf("times with until = %+v", se)
	}
}

func TestViewFrameAndSheetRenderTheCurrentRevision(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable() // one text item of 1 s at 30 fps, revision 2
	ctx := m.as(ui)

	f, err := m.svc.ViewFrame(ctx, service.ViewFrameRequest{ProjectID: project, At: "0.5s"})
	if err != nil {
		t.Fatal(err)
	}

	if f.Mode != "frame" || f.Revision != 2 || f.MIME != "image/jpeg" ||
		!bytes.HasPrefix(f.Image, []byte{0xff, 0xd8}) ||
		f.ImageTokens > 800 ||
		len(f.Cells) != 1 ||
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

	if s.Mode != "sheet" || len(s.Cells) != 4 || s.ImageTokens > 1500 ||
		!bytes.HasPrefix(s.Image, []byte{0xff, 0xd8}) ||
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

// The plan of a window hands out bitmap keys, and each key is served as a PNG.
func TestPlanWindowAndBitmapKeys(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable() // one text of 1 s at 30 fps, revision 2
	ctx := m.as(ui)

	w, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: project, From: 0, Until: 30})
	if err != nil {
		t.Fatal(err)
	}

	if w.Revision != 2 || w.EndFrame != 30 || w.Width != 1080 || len(w.Layers) != 1 || w.Layers[0].Bitmaps == nil ||
		len(w.Layers[0].Bitmaps.Keys) == 0 || len(w.Layers[0].Bitmaps.Index) != 30 || len(w.Audio) != 0 {
		t.Fatalf("window = %+v", w)
	}

	f, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: project, ID: w.Layers[0].Bitmaps.Keys[0]})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(m.ws, filepath.FromSlash(f.Path)))
	if err != nil || !bytes.HasPrefix(raw, []byte("\x89PNG")) || f.ContentType != "image/png" || !f.Immutable ||
		f.ETag != `"bmp-`+w.Layers[0].Bitmaps.Keys[0]+`"` || f.Size != int64(len(raw)) {
		t.Errorf("file = %+v, %v", f, err)
	}

	for name, key := range map[string]string{"unknown": strings.Repeat("a", 64), "not a key": "../../db.sqlite"} {
		if _, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: project, ID: key}); err == nil {
			t.Errorf("%s: served", name)
		}
	}

	for name, req := range map[string]service.PlanWindowRequest{
		"empty":    {ProjectID: project, From: 5, Until: 5},
		"backward": {ProjectID: project, From: 9, Until: 3},
		"too long": {ProjectID: project, From: 0, Until: 301},
		"negative": {ProjectID: project, From: -1, Until: 3},
	} {
		if _, err := m.svc.PlanWindow(ctx, req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A bitmap key is content-addressed, not a secret: only the project whose plan
// handed it out can read it (SDD-003 section 6.5).
func TestSecurity_BitmapKeyIsScopedToItsProject(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	a := m.exportable()
	b := m.exportable() // the same content, so the same key, but never planned

	ctx := m.as(ui)

	w, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: a, From: 0, Until: 30})
	if err != nil {
		t.Fatal(err)
	}

	key := w.Layers[0].Bitmaps.Keys[0]

	if _, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: a, ID: key}); err != nil {
		t.Fatalf("own project: %v", err)
	}

	_, err = m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: b, ID: key})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != "E_NOT_FOUND" {
		t.Errorf("key of project A on project B = %v, want E_NOT_FOUND", err)
	}

	limited := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k1", Scopes: service.AllScopes, ProjectID: b},
	)

	_, err = m.svc.OpenFile(limited, service.FileRef{Kind: "bitmap", ProjectID: a, ID: key})
	if !errors.As(err, &se) || se.Code != service.CodeForbidden {
		t.Errorf("key limited to B on project A = %v, want forbidden", err)
	}
}

// VV-037: a token budget over what a 1080 pixel wide frame costs gives the widest frame, not a refusal.
func TestViewFrameAcceptsTheWholeTokenRange(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable()
	m.mustEdit(ui, project, m.head(project).Revision, op("settings.set", `{"width":1920,"height":1080}`))

	for _, tokens := range []int{800, 1500, 1568} {
		f, err := m.svc.ViewFrame(m.as(ui), service.ViewFrameRequest{ProjectID: project, At: "0s", MaxTokens: tokens})
		if err != nil {
			t.Fatalf("max_tokens %d: %v", tokens, err)
		}

		if f.Width > 1080 || f.ImageTokens > tokens {
			t.Errorf("max_tokens %d: %dx%d, %d tokens", tokens, f.Width, f.Height, f.ImageTokens)
		}
	}
}

// VV-037: a png export is one lossless frame at the project size.
func TestExportStillIsAPNGAtTheProjectSize(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true})
	project := m.exportable()
	m.mustEdit(ui, project, m.head(project).Revision, op("settings.set", `{"width":320,"height":180}`))

	start := func(args string) (*service.Job, error) {
		return m.svc.JobStart(m.as(ui), service.JobStartRequest{
			ProjectID: project, Kind: "export", Args: []byte(args),
		})
	}

	job, err := start(`{"format":"png","at":"0.5s"}`)
	if err != nil || job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
		t.Fatalf("png export: %v, %+v", err, job)
	}

	e := job.Result.Export

	body, rerr := os.ReadFile(e.Path)
	if rerr != nil || !strings.HasSuffix(e.Path, ".png") || e.Format != "png" || e.Width != 320 || e.Height != 180 ||
		!bytes.HasPrefix(body, []byte("\x89PNG")) || e.Bytes != int64(len(body)) {
		t.Errorf("result %+v, read error %v", e, rerr)
	}

	for name, args := range map[string]string{
		"at on a video":  `{"at":"1s"}`,
		"range on a png": `{"format":"png","start":"0s"}`,
		"past the end":   `{"format":"png","at":"9s"}`,
		"not a time":     `{"format":"png","at":"soon"}`,
	} {
		if _, err := start(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
