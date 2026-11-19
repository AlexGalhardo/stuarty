package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/motion"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// MotionService is the hand-off from an external motion engine (SDD-011 section 15): what a render must fit,
// whether a file does and the replacement of a clip's media. The rules are in core/motion; the product runs no
// engine.
type MotionService interface {
	MotionBrief(ctx context.Context, req MotionBriefRequest) (*MotionBriefResult, error)
	MotionCheck(ctx context.Context, req MotionCheckRequest) (*MotionCheckResult, error)
	MotionReplace(ctx context.Context, req MotionReplaceRequest) (*MotionReplaceResult, error)
}

var _ MotionService = (*svc)(nil)

// motionFrameBudget is the image budget of a reference frame, in tokens.
const motionFrameBudget = 800

// maxMotionCaptions keeps the answer inside the budget of a tool result.
const maxMotionCaptions = 200

// MotionBriefRequest asks for the brief of a range of a timeline.
type MotionBriefRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	From       string `json:"from,omitempty"`        // a time; default 0s
	Until      string `json:"until,omitempty"`       // a time, exclusive; default the end of the timeline
	TrackID    string `json:"track_id,omitempty"`    // the visual track the clip goes on
}

// MotionBrand is the part of the brand kit a render needs: names and values, never a file.
type MotionBrand struct {
	Logo   string            `json:"logo,omitempty"`   // the library hash of the logo asset
	Colors map[string]string `json:"colors,omitempty"` // name to #RRGGBB
	Fonts  []string          `json:"fonts,omitempty"`  // family names
}

// MotionCaption is a caption inside the range; Start and End are frames relative to the start of the range.
type MotionCaption struct {
	ItemID string `json:"item_id"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Text   string `json:"text"    untrusted:"true"`
}

// MotionFrame is a reference picture of the composed timeline just outside the range.
type MotionFrame struct {
	Role        string    `json:"role"` // before or after
	At          TimeValue `json:"at"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	ImageTokens int       `json:"image_tokens"`
	MIME        string    `json:"mime"`
	// Path is the cached file for a local caller; any other caller gets the bytes.
	Path  string `json:"path,omitempty"`
	Image []byte `json:"image_base64,omitempty" bulk:"true"`
}

// MotionBriefResult is what an external render must fit (SDD-011 section 15.1).
type MotionBriefResult struct {
	Envelope
	ProjectID      string            `json:"project_id"`
	Revision       int64             `json:"revision"`
	From           TimeValue         `json:"from"`
	Until          TimeValue         `json:"until"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	FrameRate      timeline.Rational `json:"frame_rate"`
	DurationFrames int               `json:"duration_frames"`
	Duration       TimeValue         `json:"duration"`
	SafeArea       motion.Rect       `json:"safe_area"`
	Margins        motion.Margins    `json:"margins"`
	Track          *motion.TrackInfo `json:"track,omitempty"`
	NeedsAlpha     bool              `json:"needs_alpha"`
	BrandKit       MotionBrand       `json:"brand_kit"`
	Captions       []MotionCaption   `json:"captions"`
	Frames         []MotionFrame     `json:"frames"`
	// ImageTokens is the total of the reference frames.
	ImageTokens int    `json:"image_tokens"`
	Text        string `json:"text"`
}

// motionView is the project at one revision, read once for a brief or a check.
type motionView struct {
	caller Caller
	row    ProjectRow
	doc    *timeline.Document
	fps    timeval.FPS
	from   int
	until  int
}

// beginMotion reads the project and the range the request names.
func (s *svc) beginMotion(ctx context.Context, method string, req *MotionBriefRequest) (*motionView, error) {
	c, err := authorize(ctx, method, req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	row, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	v := &motionView{caller: c, row: row, doc: doc, fps: timeval.FPS{
		Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den),
	}}

	if v.from, err = v.parse("from", req.From, 0); err != nil {
		return nil, err
	}

	if v.until, err = v.parse("until", req.Until, int(endFrame(doc))); err != nil {
		return nil, err
	}

	return v, nil
}

// parse reads a time of the range; an empty text is the default.
func (v *motionView) parse(field, raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}

	t, perr := timeval.Parse(raw, v.fps)
	if perr != nil || t > int64(timeline.MaxFrames) {
		return 0, newError(command.CodeInvalidTime, "%s is not a time inside the limit of the timeline.", field).
			withPath(field).withHint("write a time like 1.5s or 45f")
	}

	return int(t), nil
}

// motionRuleError maps a refusal of core/motion to the error of the surface.
func motionRuleError(r *motion.RuleError) error {
	switch r.Reason {
	case "range_too_long":
		return newError("E_LIMIT_EXCEEDED", "%s.", r.Detail).withPath(r.Path).
			withDetail("limit", "duration_frames").withDetail("ceiling", motion.MaxFrames).
			withHint("ask for a range of at most %d frames", motion.MaxFrames)
	case "track_unknown":
		return newError(command.CodeNotFound, "%s.", r.Detail).withPath(r.Path).
			withHint("use the id of a track listed by project_get")
	case "track_not_visual":
		return newError(command.CodeTypeMismatch, "%s.", r.Detail).withPath(r.Path).
			withHint("use a visual track")
	}

	return newError(command.CodeInvalidTime, "%s.", r.Detail).withPath(r.Path).
		withHint("give a from before until, in a project with a timeline length of at least one frame")
}

// MotionBrief returns what a render by another tool must fit for a range of a timeline: size, frame rate, length,
// the safe area of the format, the brand kit, the captions inside the range and the frames on either side of it
// (SDD-011 section 15.1). It changes nothing and renders at most two frames.
func (s *svc) MotionBrief(ctx context.Context, req MotionBriefRequest) (*MotionBriefResult, error) {
	v, err := s.beginMotion(ctx, "MotionBrief", &req)
	if err != nil {
		return nil, err
	}

	b, rerr := motion.Build(v.doc, v.from, v.until, req.TrackID)
	if rerr != nil {
		return nil, motionRuleError(rerr)
	}

	fps := v.doc.Settings.FPS

	res := &MotionBriefResult{
		UntrustedNotice: UntrustedNotice, ProjectID: v.row.ID, Revision: v.row.Revision,
		From: timeValue(fps, int64(b.FromFrame)), Until: timeValue(fps, int64(b.UntilFrame)),
		Width: b.Width, Height: b.Height, FrameRate: b.FrameRate, DurationFrames: b.DurationFrames,
		Duration: timeValue(fps, int64(b.DurationFrames)), SafeArea: b.SafeArea, Margins: b.Margins,
		Track: b.Track, NeedsAlpha: b.NeedsAlpha, Captions: []MotionCaption{}, Frames: []MotionFrame{},
	}

	for _, c := range b.Captions[:min(len(b.Captions), maxMotionCaptions)] {
		res.Captions = append(res.Captions, MotionCaption{ItemID: c.ItemID, Start: c.Start, End: c.End, Text: c.Text})
	}

	if res.BrandKit, err = s.motionBrand(ctx, v.caller); err != nil {
		return nil, err
	}

	if err := s.motionFrames(ctx, v, &req, res); err != nil {
		return nil, err
	}

	res.Text = motionText(res, v.fps, len(b.Captions))

	return res, nil
}

// motionBrand reads the brand kit of the caller; a key limited to one project gets none (SDD-005 section 5.11).
func (s *svc) motionBrand(ctx context.Context, c Caller) (MotionBrand, error) {
	if c.ProjectID != "" {
		return MotionBrand{}, nil
	}

	kit, lerr := s.loadBrandKit(ctx, s.store, c)
	if lerr != nil {
		return MotionBrand{}, lerr
	}

	out := MotionBrand{Logo: kit.Logo, Colors: kit.Colors}
	for family := range kit.Fonts {
		out.Fonts = append(out.Fonts, family)
	}

	sort.Strings(out.Fonts)

	return out, nil
}

// motionFrames adds the frame just before the range and the frame at its end, as the timeline shows them. A role
// whose frame lies outside the timeline, and every role in a build that cannot render, is left out.
func (s *svc) motionFrames(ctx context.Context, v *motionView, req *MotionBriefRequest, res *MotionBriefResult) error {
	if s.media.Frames == nil {
		return nil
	}

	last := max(int(endFrame(v.doc))-1, 0)

	for _, at := range []struct {
		role  string
		frame int
		ok    bool
	}{
		{"before", v.from - 1, v.from >= 1 && v.from-1 <= last},
		{"after", v.until, v.until <= last},
	} {
		if !at.ok {
			continue
		}

		shot, err := s.ViewFrame(ctx, ViewFrameRequest{
			ProjectID: req.ProjectID,
			At:        strconv.Itoa(at.frame) + "f",
			MaxTokens: motionFrameBudget,
			Pin:       v.row.Revision,
		})
		if err != nil {
			return err
		}

		f := MotionFrame{
			Role: at.role, At: timeValue(v.doc.Settings.FPS, int64(at.frame)), Width: shot.Width, Height: shot.Height,
			ImageTokens: shot.ImageTokens, MIME: shot.MIME, Path: shot.Path,
		}
		if f.Path == "" {
			f.Image = shot.Image
		}

		res.Frames = append(res.Frames, f)
		res.ImageTokens += f.ImageTokens
	}

	return nil
}

// motionText is the brief for an agent. The captions are text from the project and are delimited.
func motionText(r *MotionBriefResult, fps timeval.FPS, captions int) string {
	var sb strings.Builder

	sb.WriteString(textview.Note + "\n")
	fmt.Fprintf(
		&sb,
		"motion brief for %s r%d: range %s to %s (%d frames, %s), render at %dx%d, %d/%d fps\n",
		r.ProjectID,
		r.Revision,
		textview.Cell(fps, r.From.Frame),
		textview.Cell(fps, r.Until.Frame),
		r.DurationFrames,
		textview.Cell(fps, int64(r.DurationFrames)),
		r.Width,
		r.Height,
		r.FrameRate.Num,
		r.FrameRate.Den,
	)
	fmt.Fprintf(&sb, "safe area x=%d y=%d width=%d height=%d (margins left %d, right %d, top %d, bottom %d)\n",
		r.SafeArea.X, r.SafeArea.Y, r.SafeArea.Width, r.SafeArea.Height,
		r.Margins.Left, r.Margins.Right, r.Margins.Top, r.Margins.Bottom)

	switch {
	case r.Track != nil && r.NeedsAlpha:
		fmt.Fprintf(
			&sb,
			"track %s is above another visual track: render with transparency (ProRes 4444 in a MOV, or WebM VP9 with alpha)\n",
			r.Track.ID,
		)
	case r.Track != nil:
		fmt.Fprintf(&sb, "track %s is the lowest visual track: an opaque render is enough\n", r.Track.ID)
	default:
		sb.WriteString("no track given: render with transparency if the clip will go over another one\n")
	}

	var kit []string
	if r.BrandKit.Logo != "" {
		kit = append(kit, "logo "+r.BrandKit.Logo)
	}

	names := make([]string, 0, len(r.BrandKit.Colors))
	for n := range r.BrandKit.Colors {
		names = append(names, n)
	}

	sort.Strings(names)

	for _, n := range names {
		kit = append(kit, n+"="+r.BrandKit.Colors[n])
	}

	if len(r.BrandKit.Fonts) > 0 {
		kit = append(kit, "fonts "+strings.Join(r.BrandKit.Fonts, ", "))
	}

	if len(kit) == 0 {
		kit = []string{"none set"}
	}

	sb.WriteString("brand kit: " + strings.Join(kit, "; ") + "\n")

	fmt.Fprintf(&sb, "captions in the range: %d\n", captions)

	for _, c := range r.Captions {
		sb.WriteString(untrusted.Block(
			untrusted.SourceProjectText,
			untrusted.Attrs{
				"item":  c.ItemID,
				"start": strconv.Itoa(c.Start),
				"end":   strconv.Itoa(c.End),
			},
			c.Text,
			0,
		) + "\n")
	}

	for _, f := range r.Frames {
		fmt.Fprintf(
			&sb,
			"reference frame %s at %s: %dx%d jpeg%s\n",
			f.Role,
			textview.Cell(fps, f.At.Frame),
			f.Width,
			f.Height,
			saved(f.Path),
		)
	}

	return strings.TrimRight(sb.String(), "\n")
}
