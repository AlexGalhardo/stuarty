// Package templates embeds the starter templates every workspace starts with
// (ADR-0022 item 8). The documents are built from commands by the generator test
// in starter/, never written by hand.
package templates

import (
	"embed"
	"fmt"
)

//go:embed starter/*.template.json
var files embed.FS

// Starter is one embedded template: a fixed identifier, so that seeding a
// workspace twice finds the row it made, and the canonical document.
type Starter struct {
	ID       string
	Document []byte
}

// starters lists the files with their identifiers (10 Crockford characters).
var starters = []struct{ id, file string }{
	{"tpl_5tar7c0001", "vertical-cut-captions.template.json"},
	{"tpl_5tar7c0002", "product-teaser.template.json"},
	{"tpl_5tar7c0003", "quote-card.template.json"},
}

// Starters returns the three starter templates in a fixed order.
func Starters() ([]Starter, error) {
	out := make([]Starter, 0, len(starters))

	for _, s := range starters {
		data, err := files.ReadFile("starter/" + s.file)
		if err != nil {
			return nil, fmt.Errorf("templates: %w", err)
		}

		out = append(out, Starter{ID: s.id, Document: data})
	}

	return out, nil
}
