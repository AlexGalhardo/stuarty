// Package templates holds the templates that ship with the recipes of Stuarty (SDD-018 section 17, epic 39): the
// timeline templates of a product commercial and of the opening of a long video, and the matching image
// templates for the cover. They are documents without media, built from commands by the generator test of this
// folder (never written by hand) and embedded in the binary. The service installs them as built-in templates
// (service.SeedBuiltInTemplates); this package knows nothing of the service.
package templates

import (
	"embed"
	"fmt"
)

//go:embed timeline/*.template.json image/*.image.json
var files embed.FS

// Template is one embedded template: a fixed identifier, so that installing a workspace twice finds the row it
// made, and the canonical document.
type Template struct {
	ID       string
	Name     string // the file name without its suffix, which the skills and the scripts use
	Document []byte
}

type entry struct{ id, name, dir, suffix string }

// The identifiers are 10 Crockford characters after the prefix (SDD-000), fixed forever: a template that changes
// keeps its identifier.
var timelines = []entry{
	{"tpl_r3c1p30001", "commercial-15s", "timeline", ".template.json"},
	{"tpl_r3c1p30002", "commercial-30s", "timeline", ".template.json"},
	{"tpl_r3c1p30003", "long-video-opening", "timeline", ".template.json"},
}

var images = []entry{
	{"img_r3c1p30001", "commercial-cover-vertical", "image", ".image.json"},
	{"img_r3c1p30002", "commercial-cover-landscape", "image", ".image.json"},
	{"img_r3c1p30003", "long-video-thumbnail", "image", ".image.json"},
}

func read(list []entry) ([]Template, error) {
	out := make([]Template, 0, len(list))

	for _, e := range list {
		data, err := files.ReadFile(e.dir + "/" + e.name + e.suffix)
		if err != nil {
			return nil, fmt.Errorf("templates: %w", err)
		}

		out = append(out, Template{ID: e.id, Name: e.name, Document: data})
	}

	return out, nil
}

// Timelines returns the timeline templates, in a fixed order.
func Timelines() ([]Template, error) { return read(timelines) }

// Images returns the image templates, in a fixed order.
func Images() ([]Template, error) { return read(images) }
