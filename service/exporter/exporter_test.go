package exporter

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
)

func TestExportRefusesWhatItCannotDo(t *testing.T) {
	doc := &timeline.Document{Items: map[string]timeline.Item{"itm_a": {Start: 0, Duration: 30}}}

	cases := map[string]struct {
		cfg  Config
		in   service.ExportInput
		code string
	}{
		"no runner":   {Config{}, service.ExportInput{Doc: doc}, "E_INTERNAL"},
		"no document": {Config{Runner: &ffmpeg.Runner{}}, service.ExportInput{}, "E_INTERNAL"},
		"a range start": {
			Config{Runner: &ffmpeg.Runner{}},
			service.ExportInput{Doc: doc, Start: 5, End: 30},
			"E_EXPORT_FAILED",
		},
		"a range end": {Config{Runner: &ffmpeg.Runner{}}, service.ExportInput{Doc: doc, End: 10}, "E_EXPORT_FAILED"},
	}

	for name, c := range cases {
		_, err := New(c.cfg).Export(context.Background(), c.in)

		fe, ok := errors.AsType[*ffmpeg.Error](err)
		if !ok || fe.Code != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}
}
