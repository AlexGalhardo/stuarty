package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// ExportPresetListRequest asks for the presets of SDD-004 section 8.2. Codecs also asks which codecs this
// machine can write, which runs the test encodes of SDD-004 section 8.3 the first time.
type ExportPresetListRequest struct {
	Codecs bool `json:"codecs,omitempty"`
}

// ExportPresetInfo is one preset as a person or an agent reads it.
type ExportPresetInfo struct {
	ID        string              `json:"id"`
	Network   string              `json:"network"`
	Placement string              `json:"placement"`
	Label     string              `json:"label"`
	Container string              `json:"container"`
	Codecs    []string            `json:"codecs"`
	Limits    ExportPresetLimits  `json:"limits"`
	Targets   ExportPresetTargets `json:"targets"`
	Sources   []exportcore.Source `json:"sources"`
}

// ExportPresetLimits are the limits of a preset in units a reader does not have to convert. A field that
// is absent is a limit the source page does not state.
type ExportPresetLimits struct {
	MinDurationMS int64  `json:"min_duration_ms,omitempty"`
	MaxDurationMS int64  `json:"max_duration_ms,omitempty"`
	MinFPS        int    `json:"min_fps,omitempty"`
	MaxFPS        int    `json:"max_fps,omitempty"`
	MinWidth      int    `json:"min_width,omitempty"`
	MinHeight     int    `json:"min_height,omitempty"`
	MaxWidth      int    `json:"max_width,omitempty"`
	MaxHeight     int    `json:"max_height,omitempty"`
	AspectRatio   string `json:"aspect_ratio,omitempty"`
	MaxBytes      int64  `json:"max_bytes,omitempty"`
	MaxVideoKbps  int    `json:"max_video_kbps,omitempty"`
}

// ExportPresetTargets are what an export with the preset asks of the encoders.
type ExportPresetTargets struct {
	Width     int `json:"width"`
	Height    int `json:"height"`
	VideoKbps int `json:"video_kbps"`
	AudioKbps int `json:"audio_kbps"`
}

// CodecAvailability says whether this machine can write a codec, and with what.
type CodecAvailability struct {
	Codec     string `json:"codec"`
	Available bool   `json:"available"`
	Encoder   string `json:"encoder,omitempty"`
	// Reason is why the codec is not available: the answer of each candidate, with no path.
	Reason string `json:"reason,omitempty"`
}

// ExportPresetListResult is the list of presets.
type ExportPresetListResult struct {
	Presets []ExportPresetInfo `json:"presets"`
	// Codecs is present when the request asked for it and the exporter can tell.
	Codecs []CodecAvailability `json:"codecs,omitempty"`
	Text   string              `json:"text"`
}

// CodecReporter is implemented by an Exporter that can say which codecs the machine writes.
type CodecReporter interface {
	Codecs(ctx context.Context) []CodecAvailability
}

// ExportPresetList returns the presets, and with Codecs the codecs this machine offers.
func (s *svc) ExportPresetList(ctx context.Context, req ExportPresetListRequest) (*ExportPresetListResult, error) {
	if _, err := authorize(ctx, "ExportPresetList", ""); err != nil {
		return nil, err
	}

	res := &ExportPresetListResult{Presets: []ExportPresetInfo{}}
	all := exportcore.Presets()
	for i := range all {
		res.Presets = append(res.Presets, presetInfo(&all[i]))
	}

	if req.Codecs {
		if rep, ok := s.exporter.(CodecReporter); ok && s.exporter != nil {
			res.Codecs = rep.Codecs(ctx)
		}
	}

	res.Text = presetListText(res)

	return res, nil
}

func presetInfo(p *exportcore.Preset) ExportPresetInfo {
	l := p.Limits
	info := ExportPresetInfo{
		ID: p.ID, Network: p.Network, Placement: p.Placement, Label: p.Label, Container: p.Container,
		Sources: p.Sources,
		Limits: ExportPresetLimits{
			MinDurationMS: l.MinDurationMS, MaxDurationMS: l.MaxDurationMS, MinFPS: l.MinFPS, MaxFPS: l.MaxFPS,
			MinWidth: l.MinWidth, MinHeight: l.MinHeight, MaxWidth: l.MaxWidth, MaxHeight: l.MaxHeight,
			AspectRatio: aspectText(l), MaxBytes: l.MaxBytes, MaxVideoKbps: l.MaxVideoKbps,
		},
		Targets: ExportPresetTargets{
			Width: p.Targets.Width, Height: p.Targets.Height, VideoKbps: p.Targets.VideoKbps,
			AudioKbps: p.Targets.AudioKbps,
		},
	}

	for i := range p.Codecs {
		info.Codecs = append(info.Codecs, string(p.Codecs[i]))
	}

	return info
}

func aspectText(l exportcore.Limits) string {
	low, high := l.AspectMin, l.AspectMax

	switch {
	case low.Num > 0 && low == high:
		return fmt.Sprintf("%d:%d", low.Num, low.Den)
	case low.Num > 0 && high.Num > 0:
		return fmt.Sprintf("%d:%d to %d:%d", low.Num, low.Den, high.Num, high.Den)
	case high.Num > 0:
		return fmt.Sprintf("at most %d:%d", high.Num, high.Den)
	case low.Num > 0:
		return fmt.Sprintf("at least %d:%d", low.Num, low.Den)
	}

	return ""
}

func presetListText(res *ExportPresetListResult) string {
	var b strings.Builder

	for i := range res.Presets {
		p := &res.Presets[i]
		fmt.Fprintf(&b, "%s: %s, codecs %s", p.ID, p.Label, strings.Join(p.Codecs, ", "))

		if p.Limits.MaxDurationMS > 0 {
			fmt.Fprintf(&b, ", at most %s s", strconv.FormatFloat(float64(p.Limits.MaxDurationMS)/1000, 'f', -1, 64))
		}

		if p.Limits.MinDurationMS > 0 {
			fmt.Fprintf(&b, ", at least %s s", strconv.FormatFloat(float64(p.Limits.MinDurationMS)/1000, 'f', -1, 64))
		}

		if p.Limits.AspectRatio != "" {
			fmt.Fprintf(&b, ", aspect %s", p.Limits.AspectRatio)
		}

		if p.Limits.MaxBytes > 0 {
			fmt.Fprintf(&b, ", file at most %s", decimalSize(p.Limits.MaxBytes))
		}

		if len(p.Sources) > 0 {
			fmt.Fprintf(&b, " (read %s on %s)", p.Sources[0].Read, p.Sources[0].URL)
		}

		b.WriteByte('\n')
	}

	for _, c := range res.Codecs {
		if c.Available {
			fmt.Fprintf(&b, "codec %s: available with %s\n", c.Codec, c.Encoder)
		} else {
			fmt.Fprintf(&b, "codec %s: not available on this machine\n", c.Codec)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// decimalSize writes a limit of bytes in the decimal units the networks state them in (300 MB is 300,000,000
// bytes), as the findings of the check do.
func decimalSize(n int64) string {
	if n >= 1_000_000_000 {
		return strconv.FormatFloat(float64(n)/1e9, 'f', -1, 64) + " GB"
	}

	return strconv.FormatFloat(float64(n)/1e6, 'f', -1, 64) + " MB"
}

// ExportCheckRequest asks whether a project fits a preset before anything is encoded.
type ExportCheckRequest struct {
	ProjectID string `json:"project_id"`
	Preset    string `json:"preset"`
	// Start and End are the range to export, as in an export job; both empty is the whole timeline.
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	// Codec is the codec the estimate of the size is for; empty is the first codec of the preset.
	Codec string `json:"codec,omitempty"`
}

// ExportCheckResult is what ExportCheck found, with the exact limit of every finding.
type ExportCheckResult struct {
	ProjectID string `json:"project_id"`
	Revision  int64  `json:"revision"`
	Preset    string `json:"preset"`
	OK        bool   `json:"ok"`
	// Errors and Warnings count the findings by severity, so the result reads like the one of Lint.
	Errors         int                  `json:"errors"`
	Warnings       int                  `json:"warnings"`
	Findings       []exportcore.Finding `json:"findings"`
	DurationMS     int64                `json:"duration_ms"`
	EstimatedBytes int64                `json:"estimated_bytes"`
	Text           string               `json:"text"`
}

// ExportCheck runs the checks of SDD-004 section 8.2 on the project as it is now.
func (s *svc) ExportCheck(ctx context.Context, req ExportCheckRequest) (*ExportCheckResult, error) {
	c, err := authorize(ctx, "ExportCheck", req.ProjectID)
	if err != nil {
		return nil, err
	}

	preset, perr := lookupPreset(req.Preset, "preset")
	if perr != nil {
		return nil, perr
	}

	codec, cerr := checkExportCodec(req.Codec, "codec")
	if cerr != nil {
		return nil, cerr
	}

	row, err := s.store.Projects().Get(ctx, req.ProjectID)
	if err != nil || !visible(c, row.OwnerUserID) {
		return nil, projectNotFound(req.ProjectID, err)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	start, end, rerr := exportRange(doc, ExportArgs{Start: req.Start, End: req.End})
	if rerr != nil {
		return nil, rerr
	}

	in := presetInput(doc, &preset, end-start, codec)
	findings := preset.Check(in)

	res := &ExportCheckResult{
		ProjectID: row.ID, Revision: row.Revision, Preset: preset.ID, OK: len(findings) == 0, Findings: findings,
		DurationMS: in.DurationMS, EstimatedBytes: preset.EstimateBytes(in),
	}
	if res.Findings == nil {
		res.Findings = []exportcore.Finding{}
	}

	res.Errors = len(res.Findings)

	res.Text = checkText(res, &preset, start, end)

	return res, nil
}

// presetInput is what Check looks at for an export of frames frames of the document.
func presetInput(doc *timeline.Document, p *exportcore.Preset, frames int64, codec exportcore.Codec) exportcore.Input {
	st := doc.Settings
	if codec == "" && len(p.Codecs) > 0 {
		codec = p.Codecs[0]
	}

	ms := int64(0)
	if st.FPS.Num > 0 {
		ms = (frames*1000*int64(st.FPS.Den) + int64(st.FPS.Num)/2) / int64(st.FPS.Num)
	}

	return exportcore.Input{
		Width: st.Width, Height: st.Height, FPSNum: st.FPS.Num, FPSDen: st.FPS.Den, DurationMS: ms, Codec: codec,
	}
}

func checkText(r *ExportCheckResult, p *exportcore.Preset, start, end int64) string {
	var b strings.Builder

	fmt.Fprintf(&b, "export check %s for %s, frames %d to %d (%.3f s), estimated %s\n",
		r.Preset, r.ProjectID, start, end, float64(r.DurationMS)/1000, textview.Size(r.EstimatedBytes))

	if r.OK {
		fmt.Fprintf(&b, "ok: the project fits %s", p.Label)

		return b.String()
	}

	for _, f := range r.Findings {
		fmt.Fprintf(&b, "%s %s: %s\n", f.Severity, f.Code, f.Message)
	}

	return strings.TrimRight(b.String(), "\n")
}

// lookupPreset finds the preset of an argument, or says which ones exist.
func lookupPreset(id, path string) (exportcore.Preset, *Error) {
	p, ok := exportcore.Lookup(id)
	if ok {
		return p, nil
	}

	return exportcore.Preset{}, newError(command.CodeValidation, "the export preset %q does not exist.", id).
		withPath("args."+path).withHint("use %s; export:presets lists them with their limits", JoinOr(exportcore.IDs()))
}

// checkExportCodec validates the codec argument of an export; empty is the choice of the preset or H.264.
func checkExportCodec(name, path string) (exportcore.Codec, *Error) {
	c := exportcore.Codec(name)
	if c.Valid() {
		return c, nil
	}

	names := make([]string, 0, 3)
	for _, k := range exportcore.Codecs() {
		names = append(names, string(k))
	}

	return "", newError(command.CodeValidation, "the codec %q is not one an export writes.", name).
		withPath("args."+path).withHint("use %s", JoinOr(names))
}

// presetMismatch is the E_VALIDATION of an export whose project does not fit its preset.
func presetMismatch(p *exportcore.Preset, findings []exportcore.Finding) *Error {
	e := newError(command.CodeValidation, "the project does not fit %s: %s", p.Label, findings[0].Message).
		withPath("args.preset").
		withHint("shorten or change the project to fit the limits in details.findings, check it again with export:check, "+
			"then export").
		withDetail("reason", "preset_mismatch").withDetail("preset", p.ID).withDetail("findings", findings)

	return e
}

// presetFit is the check of an export with a preset over a range; nil when it fits.
func presetFit(doc *timeline.Document, p *exportcore.Preset, frames int64, codec exportcore.Codec) *Error {
	if fs := p.Check(presetInput(doc, p, frames, codec)); len(fs) > 0 {
		return presetMismatch(p, fs)
	}

	return nil
}
