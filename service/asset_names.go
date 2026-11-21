package service

import (
	"context"
	"maps"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// shownAssetNames is the display name of each asset of a timeline, by asset id: the name a rename changes, which the
// document does not carry (SDD-011 section 14.8).
func (s *svc) shownAssetNames(ctx context.Context, timelineID string) (map[string]string, error) {
	rows, err := s.store.Assets().ListProjectAssets(ctx, timelineID)
	if err != nil {
		return nil, asError(err)
	}

	names := make(map[string]string, len(rows))
	for i := range rows {
		names[rows[i].AssetID] = rows[i].Name
	}

	return names, nil
}

// withShownNames is a copy of the document whose assets carry their display name, for what is read or leaves
// Stuarty by name. The document itself keeps the name it was written with.
func withShownNames(doc *timeline.Document, names map[string]string) *timeline.Document {
	shown := *doc
	shown.Assets = maps.Clone(doc.Assets)

	for id, a := range shown.Assets {
		if name := names[id]; name != "" {
			a.Name = name
			shown.Assets[id] = a
		}
	}

	return &shown
}
