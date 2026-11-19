package asset

import (
	"slices"
	"strings"
)

// Holding is one active row of project_assets: the asset as a timeline holds it.
type Holding struct {
	ProjectID  string // the production that holds the timeline
	TimelineID string
	AssetID    string // the ast_ identifier in the timeline
}

// Use is a production that uses a file.
type Use struct {
	ProjectID string   `json:"project_id"`
	Timelines []string `json:"timelines"`
	AssetID   string   `json:"asset_id"` // the identifier in the first of the timelines
}

// Uses groups the holdings by production, sorted by production and, inside it, by timeline: a production that
// holds the file in two timelines is one use (SDD-011 section 14.3).
func Uses(held []Holding) []Use {
	byProject := map[string][]Holding{}

	for _, h := range held {
		byProject[h.ProjectID] = append(byProject[h.ProjectID], h)
	}

	out := make([]Use, 0, len(byProject))

	for project, hs := range byProject {
		slices.SortFunc(hs, func(a, b Holding) int { return strings.Compare(a.TimelineID, b.TimelineID) })

		u := Use{ProjectID: project, AssetID: hs[0].AssetID}
		for _, h := range hs {
			if !slices.Contains(u.Timelines, h.TimelineID) {
				u.Timelines = append(u.Timelines, h.TimelineID)
			}
		}

		out = append(out, u)
	}

	slices.SortFunc(out, func(a, b Use) int { return strings.Compare(a.ProjectID, b.ProjectID) })

	return out
}
