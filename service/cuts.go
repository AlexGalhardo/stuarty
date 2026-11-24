package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/cuts"
	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// CutsProposeRequest asks for the vertical cuts of an asset (SDD-024 section 8).
type CutsProposeRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	AssetID    string `json:"asset_id"`
	// Count, MinMS and MaxMS override the defaults of cuts.DefaultOptions when not zero.
	Count int `json:"count,omitempty"`
	MinMS int `json:"min_ms,omitempty"`
	MaxMS int `json:"max_ms,omitempty"`
}

// CutsProposal is one proposal with the batch of commands a person accepts.
type CutsProposal struct {
	cuts.Proposal

	// Batch makes the cut on an empty vertical timeline of the project: send it to a timeline added with
	// ProjectItemAdd (kind timeline, the default 9:16 preset). The service fills the face track of the framing.
	Batch []command.Operation `json:"batch"`
}

// CutsProposeResult is the proposals; it changed nothing.
type CutsProposeResult struct {
	UntrustedNotice string         `json:"untrusted_notice,omitempty"`
	ProjectID       string         `json:"project_id"`
	Revision        int64          `json:"revision"`
	AssetID         string         `json:"asset_id"`
	Proposals       []CutsProposal `json:"proposals"`
	Text            string         `json:"text"`
}

// CutsPropose returns up to Count vertical cuts of an asset from its transcript and its face track. It reads two
// artifacts, starts no job and creates no timeline: the proposals only become timelines when a person sends the
// batch of one (SDD-024 section 8 item 5).
func (s *svc) CutsPropose(ctx context.Context, req CutsProposeRequest) (*CutsProposeResult, error) {
	src, err := s.loadTranscript(ctx, "CutsPropose", req.ProjectID, req.TimelineID, req.AssetID)
	if err != nil {
		return nil, err
	}

	opts := cuts.Options{Count: req.Count, MinMS: req.MinMS, MaxMS: req.MaxMS}.WithDefaults()
	if verr := opts.Validate(); verr != nil {
		return nil, newError(command.CodeValidation, "%s.", verr.Error()).withPath("count")
	}

	asset := src.doc.Assets[req.AssetID]
	if asset.Kind != timeline.AssetVideo {
		return nil, newError(command.CodeTypeMismatch, "asset %s is not a video.", req.AssetID).withPath("asset_id")
	}

	arts, aerr := s.store.Assets().Artifacts(ctx, asset.Hash)
	if aerr != nil {
		return nil, asError(aerr)
	}

	track := s.newestFaceTrack(arts)
	if track == nil {
		return nil, newError(command.CodeInvalidState, "asset %s has no face track yet.", req.AssetID).
			withPath("asset_id").
			withHint("run a face_track job for this asset (job_start kind face_track), wait for it to succeed, then ask again")
	}

	words := make([]cuts.Word, len(src.spoken))
	for i, w := range src.spoken {
		words[i] = cuts.Word{Text: w.Text, StartMS: int(w.StartMS), EndMS: int(w.EndMS)}
	}

	found := cuts.Propose(words, track, opts)
	res := &CutsProposeResult{
		UntrustedNotice: UntrustedNotice, ProjectID: src.row.ID, Revision: src.row.Revision, AssetID: req.AssetID,
		Proposals: make([]CutsProposal, 0, len(found)),
	}

	for i := range found {
		batch, berr := cuts.Batch(&found[i], &asset)
		if berr != nil {
			return nil, internalError()
		}

		found[i].Title = untrusted.Clean(found[i].Title)
		res.Proposals = append(res.Proposals, CutsProposal{Proposal: found[i], Batch: batch})
	}

	res.Text = cutsText(res)

	return res, nil
}

func cutsText(r *CutsProposeResult) string {
	var b strings.Builder

	b.WriteString(textview.Note + "\n")
	fmt.Fprintf(&b, "%d vertical cuts of %s at r%d; nothing was changed", len(r.Proposals), r.AssetID, r.Revision)

	for i := range r.Proposals {
		p := &r.Proposals[i]
		fmt.Fprintf(&b, "\n%d: %s to %s, %d words, face %d%%: %s",
			p.N, msText(p.StartMS), msText(p.EndMS), p.Words, p.FaceShare/10, p.Title)
	}

	return b.String()
}

func msText(ms int) string { return fmt.Sprintf("%d:%02d", ms/60000, ms/1000%60) }

// newestFaceTrack is the track of the highest rate, then the widest range, among the face tracks stored for the
// asset; nil when there is none.
func (s *svc) newestFaceTrack(arts []ArtifactRow) *facetrack.Track {
	var best *facetrack.Track

	for i := range arts {
		if arts[i].Kind != faceArtifactKind || arts[i].Ext != "json" {
			continue
		}

		raw := s.readArtifact(arts[i:i+1], faceArtifactKind, arts[i].Variant)

		var t facetrack.Track
		if raw == nil || json.Unmarshal(raw, &t) != nil || t.Validate() != nil {
			continue
		}

		if best == nil || t.EndMS-t.StartMS > best.EndMS-best.StartMS ||
			t.EndMS-t.StartMS == best.EndMS-best.StartMS && t.FPS > best.FPS {
			best = &t
		}
	}

	return best
}
