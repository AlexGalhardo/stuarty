package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/motion"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// MotionCheckRequest asks whether an asset fits the brief of a range of a timeline: the request of the brief and the
// asset.
type MotionCheckRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	AssetID    string `json:"asset_id"`
	From       string `json:"from,omitempty"`     // a time; default 0s
	Until      string `json:"until,omitempty"`    // a time, exclusive; default the end of the timeline
	TrackID    string `json:"track_id,omitempty"` // the visual track the clip goes on
}

// MotionCheckResult lists each way a file does not fit the brief, with the value expected (SDD-011 section 15.3).
type MotionCheckResult struct {
	Envelope
	ProjectID string `json:"project_id"`
	Revision  int64  `json:"revision"`
	AssetID   string `json:"asset_id"`
	AssetName string `json:"asset_name" untrusted:"true"`
	// OK is true when the asset fits the brief.
	OK         bool              `json:"ok"`
	Mismatches []motion.Mismatch `json:"mismatches"`
	// The brief the file was compared with.
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	FrameRate      timeline.Rational `json:"frame_rate"`
	DurationFrames int               `json:"duration_frames"`
	NeedsAlpha     bool              `json:"needs_alpha"`
	Text           string            `json:"text"`
}

// MotionCheck compares an asset of the project with the brief of a range and reports each mismatch of size, frame
// rate, length and alpha with the exact value expected. It changes nothing and stretches nothing.
func (s *svc) MotionCheck(ctx context.Context, req MotionCheckRequest) (*MotionCheckResult, error) {
	v, err := s.beginMotion(ctx, "MotionCheck", &MotionBriefRequest{
		ProjectID: req.ProjectID, TimelineID: req.TimelineID, From: req.From, Until: req.Until,
	})
	if err != nil {
		return nil, err
	}

	if !timeline.ValidID(timeline.PrefixAsset, req.AssetID) {
		return nil, newError(command.CodeValidation, "the asset id is not valid.").withPath("asset_id").
			withHint("an asset id is ast_ and 10 characters from 0-9 and a-z, without i, l, o and u")
	}

	b, rerr := motion.Build(v.doc, v.from, v.until, req.TrackID)
	if rerr != nil {
		return nil, motionRuleError(rerr)
	}

	a, ok := v.doc.Assets[req.AssetID]
	if !ok {
		return nil, s.motionAssetMissing(ctx, v, req.AssetID)
	}

	res := &MotionCheckResult{
		UntrustedNotice: UntrustedNotice, ProjectID: v.row.ID, Revision: v.row.Revision,
		AssetID: a.ID, AssetName: a.Name, Mismatches: motion.Check(b, a), Width: b.Width, Height: b.Height,
		FrameRate: b.FrameRate, DurationFrames: b.DurationFrames, NeedsAlpha: b.NeedsAlpha,
	}
	res.OK = len(res.Mismatches) == 0
	res.Text = motionCheckText(res)

	return res, nil
}

// motionAssetMissing is the refusal for an asset the document does not hold: one that is still being prepared is
// not there yet, any other is not the project's.
func (s *svc) motionAssetMissing(ctx context.Context, v *motionView, assetID string) error {
	if row, err := s.store.Assets().
		GetProjectAsset(ctx, v.row.ID, assetID); err == nil &&
		row.State != ProjectAssetActive {
		return newError("E_ASSET_NOT_READY", "asset %s is %s, not in the project yet.", assetID, row.State).
			withPath("asset_id").withHint("wait for the import to finish; media:inspect tells its state")
	}

	return newError(command.CodeNotFound, "asset %s is not in the project.", assetID).withPath("asset_id").
		withHint("import the file first, then use its asset id")
}

func motionCheckText(r *MotionCheckResult) string {
	var sb strings.Builder

	sb.WriteString(textview.Note + "\n")

	verdict := "fits the brief"
	if !r.OK {
		verdict = fmt.Sprintf("does not fit the brief: %d mismatch(es)", len(r.Mismatches))
	}

	fmt.Fprintf(
		&sb,
		"asset %s %s of %s r%d (%dx%d, %d/%d fps, %d frames%s)\n",
		r.AssetID,
		verdict,
		r.ProjectID,
		r.Revision,
		r.Width,
		r.Height,
		r.FrameRate.Num,
		r.FrameRate.Den,
		r.DurationFrames,
		map[bool]string{true: ", needs alpha"}[r.NeedsAlpha],
	)

	for _, m := range r.Mismatches {
		fmt.Fprintf(&sb, "- %s: expected %s, found %s; %s\n", m.Field, m.Expected, m.Found, m.Hint)
	}

	sb.WriteString(untrusted.Block(untrusted.SourceAssetName, untrusted.Attrs{"asset": r.AssetID}, r.AssetName, 0))

	return sb.String()
}
