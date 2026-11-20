package main

import (
	"github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/facebook"
	"github.com/AlexGalhardo/stuarty/service/social/instagram"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
	"github.com/AlexGalhardo/stuarty/service/social/tiktok"
	"github.com/AlexGalhardo/stuarty/service/social/x"
	"github.com/AlexGalhardo/stuarty/service/social/youtube"
)

// presetDoc is the export preset a network names, in the words of the site: the limits of the video, each
// with the official page and the date it was read.
type presetDoc struct {
	ID            string      `json:"id"`
	Label         string      `json:"label"`
	Codecs        []string    `json:"codecs"`
	MinDurationMS int64       `json:"min_duration_ms,omitempty"`
	MaxDurationMS int64       `json:"max_duration_ms,omitempty"`
	MinFPS        int         `json:"min_fps,omitempty"`
	MaxFPS        int         `json:"max_fps,omitempty"`
	MinWidth      int         `json:"min_width,omitempty"`
	MinHeight     int         `json:"min_height,omitempty"`
	MaxWidth      int         `json:"max_width,omitempty"`
	MaxHeight     int         `json:"max_height,omitempty"`
	MaxBytes      int64       `json:"max_bytes,omitempty"`
	TargetWidth   int         `json:"target_width"`
	TargetHeight  int         `json:"target_height"`
	Sources       []sourceDoc `json:"sources"`
}

type sourceDoc struct {
	URL  string `json:"url"`
	Read string `json:"read"`
	Says string `json:"says"`
}

type networkDoc struct {
	social.Description
	Preset *presetDoc `json:"preset,omitempty"`
}

// networkReference describes every real network of the build, in the order of the connections page. The
// connectors are made with their default address: Describe reaches no network.
func networkReference() []networkDoc {
	connectors := []social.Connector{
		youtube.New(youtube.Config{}), instagram.New(meta.Config{}), facebook.New(meta.Config{}),
		x.New(x.Config{}), tiktok.New(tiktok.Config{}),
	}

	out := make([]networkDoc, 0, len(connectors))

	for _, c := range connectors {
		d := networkDoc{Description: c.Describe()}
		d.Hints = nil

		if p, ok := export.Lookup(d.Limits.Preset); ok {
			d.Preset = presetOf(p)
		}

		out = append(out, d)
	}

	return out
}

func presetOf(p export.Preset) *presetDoc {
	doc := &presetDoc{
		ID: p.ID, Label: p.Label,
		MinDurationMS: p.Limits.MinDurationMS, MaxDurationMS: p.Limits.MaxDurationMS,
		MinFPS: p.Limits.MinFPS, MaxFPS: p.Limits.MaxFPS,
		MinWidth: p.Limits.MinWidth, MinHeight: p.Limits.MinHeight,
		MaxWidth: p.Limits.MaxWidth, MaxHeight: p.Limits.MaxHeight, MaxBytes: p.Limits.MaxBytes,
		TargetWidth: p.Targets.Width, TargetHeight: p.Targets.Height,
	}

	for _, c := range p.Codecs {
		doc.Codecs = append(doc.Codecs, string(c))
	}

	for _, s := range p.Sources {
		doc.Sources = append(doc.Sources, sourceDoc{URL: s.URL, Read: s.Read, Says: s.Says})
	}

	return doc
}
