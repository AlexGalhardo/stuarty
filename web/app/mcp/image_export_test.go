package mcp

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// exportRecorder records the request of an image export and answers a fixed result.
type exportRecorder struct {
	service.Service

	got service.ImageExportRequest
}

func (r *exportRecorder) ImageExport(
	_ context.Context, req service.ImageExportRequest,
) (*service.ImageExportResult, error) {
	r.got = req

	return &service.ImageExportResult{ImageID: req.ImageID, Format: req.Format}, nil
}

// The quality of a lossy WebP (and of a JPEG) reaches the service through the image tool (SDD-003 section 4.2).
func TestImageExportCarriesTheQuality(t *testing.T) {
	rec := &exportRecorder{Service: newService(t)}
	cs := connect(t, rec, Options{}, "test")

	ok(t, call(t, cs, "image", map[string]any{
		"action": "export", "image_id": "img_7k2m9x4q1b", "format": "webp", "quality": 60,
	}))

	if rec.got.Format != "webp" || rec.got.Quality != 60 || rec.got.ImageID != "img_7k2m9x4q1b" {
		t.Fatalf("the request was %+v", rec.got)
	}
}
